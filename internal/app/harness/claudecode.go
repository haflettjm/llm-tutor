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
const allowedTools = "mcp__llm-tutor__get_learner_context," +
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
		recovery := decideResumeRecovery(err, stderr)
		switch recovery {
		case recoveryStartFresh, recoveryStartFreshAndNotify:
			notify := recovery == recoveryStartFreshAndNotify

			c.forgetSession(req.SessionID)
			freshArgs, _ := c.sessionArgs(req.SessionID)
			out, stderr, err = c.run(ctx, freshArgs, prompt)
			if err != nil {
				return response.Response{}, fmt.Errorf("claude query after restarting session: %w: %s", err, strings.TrimSpace(stderr))
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
		return response.Response{}, fmt.Errorf("claude query: %w: %s", err, strings.TrimSpace(stderr))
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

// registerClaudeMCP writes the tutor MCP server entry into ~/.claude.json.
func registerClaudeMCP(mcpAddr string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude.json")

	var root map[string]json.RawMessage
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &root)
	}
	if root == nil {
		root = make(map[string]json.RawMessage)
	}

	servers := map[string]any{
		"llm-tutor": map[string]string{
			"type": "sse",
			"url":  "http://localhost" + mcpAddr + "/sse",
		},
	}
	if existing, ok := root["mcpServers"]; ok {
		var prev map[string]json.RawMessage
		if err := json.Unmarshal(existing, &prev); err == nil {
			for k, v := range prev {
				if k != "llm-tutor" {
					servers[k] = v
				}
			}
		}
	}

	raw, _ := json.Marshal(servers)
	root["mcpServers"] = raw
	out, _ := json.MarshalIndent(root, "", "  ")
	return os.WriteFile(path, append(out, '\n'), 0644)
}
