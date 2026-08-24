package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

type claudeCode struct {
	Base
	dataDir string

	// sessions maps an editor session ID to the Claude CLI session UUID that
	// holds its conversation. Claude owns the transcript; we only remember which
	// conversation belongs to which editor session.
	sessionsMu sync.Mutex
	sessions   map[string]string
}

func newClaudeCode(dataDir string) *claudeCode {
	return &claudeCode{
		Base:     Base{promptFile: "CLAUDE.md"},
		dataDir:  dataDir,
		sessions: make(map[string]string),
	}
}

// responseSchema is the JSON Schema Claude must match on every turn.
const responseSchema = `{"type":"object","properties":{"message":{"type":"string"},"response_type":{"type":"string","enum":["question","observation","hint","explanation"]},"hint_level":{"type":"integer","minimum":0,"maximum":3},"concept_id":{"type":"string"}},"required":["message","response_type","hint_level"]}`

// allowedTools lists every MCP tool Claude may call during a tutor turn.
const allowedTools = "mcp__llm-tutor__start_session," +
	"mcp__llm-tutor__get_learner_context," +
	"mcp__llm-tutor__set_focus," +
	"mcp__llm-tutor__set_soul," +
	"mcp__llm-tutor__set_lesson_plan," +
	"mcp__llm-tutor__update_concept_state," +
	"mcp__llm-tutor__get_next_concept," +
	"mcp__llm-tutor__list_lesson_plans," +
	"mcp__llm-tutor__update_learner_identity," +
	"mcp__llm-tutor__update_working_context," +
	"mcp__llm-tutor__update_working_style," +
	"mcp__llm-tutor__write_scratchpad," +
	"mcp__llm-tutor__end_session," +
	"mcp__llm-tutor__append_learning_event"

// resumeNotice is prepended to a reply when a resume failed and the conversation
// had to restart, so the learner understands why the tutor lost the thread.
const resumeNotice = "(I lost the thread of our earlier conversation and had to start fresh -- please re-orient me if I miss context.)\n\n"

func (c *claudeCode) SupportsResume() bool { return true }

func (c *claudeCode) Start(_ context.Context, mcpAddr string) error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude binary not found in PATH: %w", err)
	}
	if err := registerClaudeMCP(mcpAddr); err != nil {
		return fmt.Errorf("register MCP server with claude: %w", err)
	}
	c.markStarted()
	return nil
}

// sessionArgs returns the CLI flags binding this turn to a conversation, and
// whether we are resuming an existing one. The first turn of an editor session
// creates a UUID with --session-id; every later turn continues it with --resume.
func (c *claudeCode) sessionArgs(editorSession string) (args []string, resuming bool) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	if id, ok := c.sessions[editorSession]; ok {
		return []string{"--resume", id}, true
	}
	id := uuid.NewString()
	c.sessions[editorSession] = id
	return []string{"--session-id", id}, false
}

// forgetSession drops the conversation binding so the next turn starts a new one.
func (c *claudeCode) forgetSession(editorSession string) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()
	delete(c.sessions, editorSession)
}

func (c *claudeCode) Query(ctx context.Context, req request.Request) (response.Response, error) {
	prompt := buildPrompt(req)

	args, resuming := c.sessionArgs(req.SessionID)
	out, stderr, err := c.run(ctx, args, prompt)

	if err != nil && resuming {
		// The CLI reports some failures only on stdout, so both streams are
		// searched for the "session is gone" signal.
		recovery := decideResumeRecovery(err, stderr+"\n"+envelopeDetail(out))
		switch recovery {
		case recoveryStartFresh, recoveryStartFreshAndNotify:
			notify := recovery == recoveryStartFreshAndNotify

			c.forgetSession(req.SessionID)
			freshArgs, _ := c.sessionArgs(req.SessionID)
			out, stderr, err = c.run(ctx, freshArgs, prompt)
			if err != nil {
				return response.Response{}, claudeError("claude query after restarting session", err, out, stderr)
			}
			resp, parseErr := parseClaudeOutput(out)
			if parseErr != nil {
				return response.Response{}, parseErr
			}
			if notify {
				resp.Message = resumeNotice + resp.Message
			}
			return resp, nil
		case recoveryFail:
			// fall through to the error below
		}
	}

	if err != nil {
		return response.Response{}, claudeError("claude query", err, out, stderr)
	}
	return parseClaudeOutput(out)
}

// run invokes the claude CLI with the prompt on stdin. The prompt is deliberately
// NOT passed as an argv string: a single argument is capped at MAX_ARG_STRLEN
// (131072 bytes on Linux), which a large diff or a long replayed context exceeds.
func (c *claudeCode) run(ctx context.Context, sessionArgs []string, prompt string) (stdout []byte, stderr string, err error) {
	args := append([]string{
		"-p",
		"--output-format", "json",
		"--json-schema", responseSchema,
		"--allowedTools", allowedTools,
	}, sessionArgs...)

	var outBuf, errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = c.dataDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout = &outBuf
	cmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)

	err = cmd.Run()
	return outBuf.Bytes(), errBuf.String(), err
}

// claudeError builds the most informative error available for a failed run.
//
// The CLI reports model- and API-level failures as a JSON envelope on STDOUT
// and exits non-zero with nothing on stderr at all. Reporting only the exit
// status in that case produces "exit status 1: " -- an error with no
// information in it, which is the worst possible thing to hand someone at
// 2am. stdout is checked first because when it has something, it is the
// actual reason.
func claudeError(action string, err error, out []byte, stderr string) error {
	if detail := envelopeDetail(out); detail != "" {
		return fmt.Errorf("%s: %w: %s", action, err, detail)
	}
	if s := strings.TrimSpace(stderr); s != "" {
		return fmt.Errorf("%s: %w: %s", action, err, s)
	}
	return fmt.Errorf("%s: %w (the CLI wrote nothing to stdout or stderr)", action, err)
}

// envelopeDetail extracts a human-readable reason from a CLI result envelope,
// or "" when there is nothing useful in it.
func envelopeDetail(raw []byte) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	if line := bytes.LastIndexByte(raw, '\n'); line >= 0 {
		raw = bytes.TrimSpace(raw[line+1:])
	}

	var env struct {
		Type     string          `json:"type"`
		Subtype  string          `json:"subtype"`
		IsError  bool            `json:"is_error"`
		Error    string          `json:"error"`
		Terminal string          `json:"terminal_reason"`
		APIError string          `json:"api_error_status"`
		Result   json.RawMessage `json:"result"`
	}
	if json.Unmarshal(raw, &env) != nil {
		// Not JSON at all -- surface a bounded snippet rather than nothing.
		const max = 300
		if len(raw) > max {
			raw = raw[:max]
		}
		return strings.TrimSpace(string(raw))
	}

	// A run the CLI itself considers successful carries no failure detail, even
	// though the process exited non-zero for some other reason.
	if !env.IsError && (env.Subtype == "" || env.Subtype == "success") {
		return ""
	}

	var parts []string
	for _, v := range []string{env.Error, env.APIError} {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	if v := strings.TrimSpace(env.Terminal); v != "" && v != "completed" {
		parts = append(parts, v)
	}
	if env.Subtype != "" && env.Subtype != "success" {
		parts = append(parts, "subtype="+env.Subtype)
	}
	if len(parts) == 0 {
		var msg string
		if json.Unmarshal(env.Result, &msg) == nil && strings.TrimSpace(msg) != "" {
			parts = append(parts, strings.TrimSpace(msg))
		}
	}
	return strings.Join(parts, "; ")
}

// parseClaudeOutput unwraps the CLI JSON envelope and returns the typed response.
// Envelope: {"type":"result","subtype":"success","result":<json-or-string>}
func parseClaudeOutput(raw []byte) (response.Response, error) {
	raw = bytes.TrimSpace(raw)
	if line := bytes.LastIndexByte(raw, '\n'); line >= 0 {
		raw = bytes.TrimSpace(raw[line+1:])
	}

	var envelope struct {
		Type    string          `json:"type"`
		Subtype string          `json:"subtype"`
		Result  json.RawMessage `json:"result"`
		Error   string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return response.Response{}, fmt.Errorf("parse claude output: %w", err)
	}
	if envelope.Type != "result" || envelope.Subtype != "success" {
		msg := envelope.Error
		if msg == "" {
			msg = fmt.Sprintf("type=%s subtype=%s", envelope.Type, envelope.Subtype)
		}
		return response.Response{}, fmt.Errorf("claude error: %s", msg)
	}

	// --json-schema may produce a JSON object directly in result.
	var resp response.Response
	if err := json.Unmarshal(envelope.Result, &resp); err == nil {
		return resp, nil
	}

	// Fallback: result is a JSON-encoded string containing JSON.
	var str string
	if err := json.Unmarshal(envelope.Result, &str); err != nil {
		return response.Response{}, fmt.Errorf("parse claude result: unexpected format in %q", raw)
	}
	if err := json.Unmarshal([]byte(str), &resp); err != nil {
		return response.Response{}, fmt.Errorf("parse claude result json: %w", err)
	}
	return resp, nil
}

// buildPrompt composes the per-turn prompt from the editor request.
func buildPrompt(req request.Request) string {
	var sb strings.Builder
	sb.WriteString("Session: ")
	sb.WriteString(req.SessionID)
	if req.Language != "" {
		sb.WriteString("\nLanguage: ")
		sb.WriteString(req.Language)
	}
	if req.ConceptID != "" {
		sb.WriteString("\nConcept: ")
		sb.WriteString(req.ConceptID)
	}
	sb.WriteString("\n\nLearner message:\n")
	sb.WriteString(req.Message)
	if req.Diff != "" {
		sb.WriteString("\n\nDiff:\n")
		sb.WriteString(req.Diff)
	}
	return sb.String()
}

// mcpServerName is the key our entry occupies in ~/.claude.json.
const mcpServerName = "llm-tutor"

// registerClaudeMCP points Claude Code at our MCP server by writing one entry
// into ~/.claude.json.
//
// That file is shared with every Claude Code session on this machine and holds
// far more than MCP config, so this is deliberately careful: unknown keys are
// preserved verbatim as raw JSON, the write is skipped entirely when our entry
// is already correct, and the replacement goes through a temp file and a rename
// so a crash mid-write cannot truncate the learner's settings.
func registerClaudeMCP(mcpAddr string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude.json")

	root := make(map[string]json.RawMessage)
	data, readErr := os.ReadFile(path)
	if readErr == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse %s: %w -- refusing to overwrite it", path, err)
		}
	} else if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read %s: %w", path, readErr)
	}

	entry, err := json.Marshal(map[string]string{
		"type": "sse",
		"url":  mcpEndpoint(mcpAddr),
	})
	if err != nil {
		return err
	}

	servers := make(map[string]json.RawMessage)
	if existing, ok := root["mcpServers"]; ok {
		if err := json.Unmarshal(existing, &servers); err != nil {
			return fmt.Errorf("parse mcpServers in %s: %w", path, err)
		}
	}
	if cur, ok := servers[mcpServerName]; ok && sameJSON(cur, entry) {
		return nil // already registered at this address
	}
	servers[mcpServerName] = entry

	raw, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	root["mcpServers"] = raw

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".llm-tutor.tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// mcpEndpoint turns a listen address into the URL the harness dials. A bare
// ":7890" listens on every interface but must be dialled on a concrete host.
func mcpEndpoint(addr string) string {
	host, port := addr, ""
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host, port = addr[:i], addr[i+1:]
	}
	if host == "" || host == "0.0.0.0" || host == "[::]" {
		host = "localhost"
	}
	if port == "" {
		return "http://" + host + "/sse"
	}
	return "http://" + host + ":" + port + "/sse"
}

// sameJSON compares two JSON documents by value, so key order or whitespace
// differences do not trigger a pointless rewrite of a shared settings file.
func sameJSON(a, b []byte) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}
