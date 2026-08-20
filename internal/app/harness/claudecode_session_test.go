package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionArgsCreatesConversationThenResumesIt(t *testing.T) {
	c := newClaudeCode(t.TempDir())

	first, resuming := c.sessionArgs("editor-1")
	if resuming {
		t.Fatal("first turn of a session must not resume")
	}
	if first[0] != "--session-id" {
		t.Fatalf("first turn flags = %v, want --session-id", first)
	}
	id := first[1]

	second, resuming := c.sessionArgs("editor-1")
	if !resuming {
		t.Fatal("second turn must resume the existing conversation")
	}
	if second[0] != "--resume" || second[1] != id {
		t.Fatalf("second turn flags = %v, want --resume %s", second, id)
	}
}

func TestSessionArgsKeepsEditorSessionsIndependent(t *testing.T) {
	c := newClaudeCode(t.TempDir())

	a, _ := c.sessionArgs("editor-a")
	b, _ := c.sessionArgs("editor-b")

	if a[1] == b[1] {
		t.Fatalf("distinct editor sessions shared conversation %s", a[1])
	}
}

func TestForgetSessionStartsANewConversation(t *testing.T) {
	c := newClaudeCode(t.TempDir())

	first, _ := c.sessionArgs("editor-1")
	c.forgetSession("editor-1")
	next, resuming := c.sessionArgs("editor-1")

	if resuming {
		t.Fatal("after forgetting, the next turn must start fresh")
	}
	if next[1] == first[1] {
		t.Fatal("forgotten session reused the old conversation id")
	}
}

// TestRunAcceptsPromptLargerThanArgvLimit is the regression test for the E2BIG
// bug: a single argv string is capped at MAX_ARG_STRLEN (131072 bytes on Linux),
// so the prompt must travel on stdin. A real 'git diff HEAD' in this repo is
// already ~65KB, so this limit was reachable in normal use.
func TestRunAcceptsPromptLargerThanArgvLimit(t *testing.T) {
	dir := t.TempDir()
	sizeFile := filepath.Join(dir, "stdin-size")

	fake := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nwc -c > " + sizeFile + "\n" +
		`echo '{"type":"result","subtype":"success","result":"{\"message\":\"ok\",\"response_type\":\"question\",\"hint_level\":0}"}'` + "\n"
	if err := os.WriteFile(fake, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	const promptSize = 200_000 // comfortably past the 131072 argv wall
	prompt := strings.Repeat("x", promptSize)

	c := newClaudeCode(dir)
	out, stderr, err := c.run(context.Background(), []string{"--session-id", "test"}, prompt)
	if err != nil {
		t.Fatalf("run with %d byte prompt failed: %v (stderr: %s)", promptSize, err, stderr)
	}

	resp, err := parseClaudeOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message != "ok" {
		t.Fatalf("message = %q", resp.Message)
	}

	got, err := os.ReadFile(sizeFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "200000" {
		t.Fatalf("claude received %s bytes on stdin, want %d", strings.TrimSpace(string(got)), promptSize)
	}
}
