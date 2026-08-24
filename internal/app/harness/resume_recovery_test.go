package harness

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

// exitErr builds a real *exec.ExitError by running a command that fails, so the
// test exercises the same error type the CLI produces rather than a stand-in.
func exitErr(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit 1").Run()
	if err == nil {
		t.Fatal("expected a non-zero exit")
	}
	return err
}

func signalErr(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "kill -9 $$").Run()
	if err == nil {
		t.Fatal("expected a signal kill")
	}
	return err
}

// This is the exact stderr from claude 2.1.234 for a session that no longer
// exists. If a future version reworded it, this test is the tripwire.
const realMissingSessionStderr = "No conversation found with session ID: 11111111-2222-3333-4444-555555555555"

func TestPrunedSessionRestartsAndNotifies(t *testing.T) {
	got := decideResumeRecovery(exitErr(t), realMissingSessionStderr)
	if got != recoveryStartFreshAndNotify {
		t.Fatalf("recovery = %v, want recoveryStartFreshAndNotify", got)
	}
}

func TestMissingSessionWordingVariantsAllRestart(t *testing.T) {
	for _, stderr := range []string{
		"No conversation found with session ID: abc",
		"Error: session not found",
		"no such session: abc",
		"Could not find session abc",
		"Session does not exist",
		"UNKNOWN SESSION ID",
	} {
		if got := decideResumeRecovery(exitErr(t), stderr); got != recoveryStartFreshAndNotify {
			t.Errorf("stderr %q -> %v, want restart+notify", stderr, got)
		}
	}
}

// The load-bearing half of the policy: a good conversation must survive every
// failure that is not positively identified as a missing session.
func TestTransientAndOurOwnFailuresPreserveTheSession(t *testing.T) {
	for name, stderr := range map[string]string{
		"rate limit":    "Error: 429 rate_limit_error: rate limit exceeded",
		"overloaded":    "API Error: 529 overloaded_error",
		"server error":  "Error: 500 Internal Server Error",
		"network":       "TypeError: fetch failed (ECONNREFUSED 127.0.0.1:443)",
		"usage limit":   "Claude usage limit reached. Your limit will reset at 3pm.",
		"credit":        "Your credit balance is too low to access the Anthropic API.",
		"bad flag":      "error: unknown option '--not-a-real-flag'",
		"bad schema":    "Error: --json-schema must be valid JSON Schema",
		"empty":         "",
		"never seen この": "some message no version has ever printed",
	} {
		if got := decideResumeRecovery(exitErr(t), stderr); got != recoveryFail {
			t.Errorf("%s: stderr %q -> %v, want recoveryFail", name, stderr, got)
		}
	}
}

func TestCancelledTurnPreservesTheSession(t *testing.T) {
	if got := decideResumeRecovery(context.Canceled, ""); got != recoveryFail {
		t.Errorf("context.Canceled -> %v, want recoveryFail", got)
	}
	if got := decideResumeRecovery(context.DeadlineExceeded, ""); got != recoveryFail {
		t.Errorf("context.DeadlineExceeded -> %v, want recoveryFail", got)
	}
	wrapped := errors.Join(errors.New("claude query"), context.Canceled)
	if got := decideResumeRecovery(wrapped, ""); got != recoveryFail {
		t.Errorf("wrapped cancel -> %v, want recoveryFail", got)
	}
}

// A killed process says nothing about whether the conversation is still good,
// even if a stale "no conversation found" is sitting in the buffer.
func TestSignalledProcessPreservesTheSession(t *testing.T) {
	if got := decideResumeRecovery(signalErr(t), realMissingSessionStderr); got != recoveryFail {
		t.Errorf("signalled process -> %v, want recoveryFail", got)
	}
}

func TestNilErrorIsNotARecovery(t *testing.T) {
	if got := decideResumeRecovery(nil, realMissingSessionStderr); got != recoveryFail {
		t.Errorf("nil error -> %v, want recoveryFail", got)
	}
}
