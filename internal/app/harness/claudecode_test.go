package harness

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseClaudeOutputIgnoresLauncherNoise(t *testing.T) {
	raw := []byte("mise activated claude@2.1.234\n" +
		`{"type":"result","subtype":"success","result":"{\"message\":\"What do you predict?\",\"response_type\":\"question\",\"hint_level\":0}"}`)

	resp, err := parseClaudeOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message != "What do you predict?" {
		t.Fatalf("message = %q", resp.Message)
	}
}

func TestMCPEndpointResolvesListenAddressesToDialableURLs(t *testing.T) {
	for addr, want := range map[string]string{
		":7890":            "http://localhost:7890/sse",
		"0.0.0.0:7890":     "http://localhost:7890/sse",
		"127.0.0.1:7890":   "http://127.0.0.1:7890/sse",
		"localhost:7890":   "http://localhost:7890/sse",
		"tutor.local:9000": "http://tutor.local:9000/sse",
	} {
		if got := mcpEndpoint(addr); got != want {
			t.Errorf("mcpEndpoint(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestRegisterClaudeMCPPreservesOtherSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".claude.json")

	existing := `{"numStartups":42,"projects":{"/tmp/x":{"allowedTools":["Bash"]}},` +
		`"mcpServers":{"other":{"type":"stdio","command":"foo"}}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := registerClaudeMCP(":7890"); err != nil {
		t.Fatal(err)
	}

	var root map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if string(root["numStartups"]) != "42" {
		t.Errorf("numStartups = %s, want 42", root["numStartups"])
	}
	if !strings.Contains(string(root["projects"]), "allowedTools") {
		t.Errorf("projects key was lost: %s", root["projects"])
	}

	var servers map[string]json.RawMessage
	if err := json.Unmarshal(root["mcpServers"], &servers); err != nil {
		t.Fatal(err)
	}
	if _, ok := servers["other"]; !ok {
		t.Error("an unrelated MCP server was dropped")
	}
	if !strings.Contains(string(servers[mcpServerName]), "localhost:7890/sse") {
		t.Errorf("our entry = %s", servers[mcpServerName])
	}
}

func TestRegisterClaudeMCPCreatesTheFileWhenAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := registerClaudeMCP(":7890"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

// A shared settings file should not be rewritten when nothing needs to change.
func TestRegisterClaudeMCPSkipsAnAlreadyCorrectEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".claude.json")

	if err := registerClaudeMCP(":7890"); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Make a change that a rewrite would erase, then re-register.
	data, _ := os.ReadFile(path)
	marker := append([]byte("\n"), data...)
	if err := os.WriteFile(path, marker, 0600); err != nil {
		t.Fatal(err)
	}
	if err := registerClaudeMCP(":7890"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(marker) {
		t.Errorf("file was rewritten despite an identical entry")
	}
	_ = before
}

// Refusing to touch an unparseable settings file is safer than replacing it.
func TestRegisterClaudeMCPRefusesToClobberCorruptSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".claude.json")
	corrupt := "{not valid json"
	if err := os.WriteFile(path, []byte(corrupt), 0600); err != nil {
		t.Fatal(err)
	}
	if err := registerClaudeMCP(":7890"); err == nil {
		t.Fatal("expected an error rather than an overwrite")
	}
	data, _ := os.ReadFile(path)
	if string(data) != corrupt {
		t.Errorf("corrupt file was modified: %q", data)
	}
}

// The CLI reports API-level failures on stdout and exits non-zero with an empty
// stderr. Losing stdout there produces "exit status 1: " -- an error containing
// no information at all.
func TestClaudeErrorPrefersTheStdoutEnvelope(t *testing.T) {
	out := []byte(`{"type":"result","subtype":"error_during_execution","is_error":true,` +
		`"api_error_status":"429 rate_limit_error","terminal_reason":"api_error"}`)
	err := claudeError("claude query", errExit(t), out, "")

	msg := err.Error()
	if !strings.Contains(msg, "429 rate_limit_error") {
		t.Errorf("stdout diagnostic was lost: %q", msg)
	}
	if !strings.Contains(msg, "exit status") {
		t.Errorf("exit status was lost: %q", msg)
	}
}

func TestClaudeErrorFallsBackToStderr(t *testing.T) {
	err := claudeError("claude query", errExit(t), nil, "  No conversation found with session ID: abc  ")
	if !strings.Contains(err.Error(), "No conversation found") {
		t.Errorf("stderr was lost: %q", err.Error())
	}
}

// Silence must still say something actionable.
func TestClaudeErrorSaysSoWhenBothStreamsAreEmpty(t *testing.T) {
	msg := claudeError("claude query", errExit(t), nil, "").Error()
	if !strings.Contains(msg, "wrote nothing") {
		t.Errorf("empty failure was not explained: %q", msg)
	}
}

func TestEnvelopeDetailIgnoresASuccessfulRun(t *testing.T) {
	out := []byte(`{"type":"result","subtype":"success","is_error":false,"terminal_reason":"completed",` +
		`"result":"{\"message\":\"hi\",\"response_type\":\"question\",\"hint_level\":0}"}`)
	if got := envelopeDetail(out); got != "" {
		t.Errorf("a successful envelope produced a detail: %q", got)
	}
}

func TestEnvelopeDetailHandlesNonJSONOutput(t *testing.T) {
	got := envelopeDetail([]byte("Segmentation fault"))
	if !strings.Contains(got, "Segmentation fault") {
		t.Errorf("non-JSON output was dropped: %q", got)
	}
}

func TestEnvelopeDetailBoundsRunawayOutput(t *testing.T) {
	if got := envelopeDetail([]byte(strings.Repeat("x", 100_000))); len(got) > 400 {
		t.Errorf("unbounded detail of %d bytes", len(got))
	}
}

func TestEnvelopeDetailOnEmptyOutput(t *testing.T) {
	if got := envelopeDetail(nil); got != "" {
		t.Errorf("got %q", got)
	}
}

// A pruned session reported on stdout rather than stderr must still be
// recognised, which is why Query searches both streams.
func TestResumeRecoverySeesASessionErrorFromStdout(t *testing.T) {
	out := []byte(`{"type":"result","subtype":"error","is_error":true,` +
		`"error":"No conversation found with session ID: abc"}`)
	combined := "" + "\n" + envelopeDetail(out)
	if got := decideResumeRecovery(errExit(t), combined); got != recoveryStartFreshAndNotify {
		t.Fatalf("recovery = %v, want restart+notify", got)
	}
}

func errExit(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit 1").Run()
	if err == nil {
		t.Fatal("expected a non-zero exit")
	}
	return err
}
