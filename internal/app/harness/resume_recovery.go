package harness

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// resumeRecovery is what the tutor does when it tries to continue an existing
// conversation and the CLI refuses.
type resumeRecovery int

const (
	// recoveryFail surfaces the error and ends the turn. The learner sees an
	// error instead of a reply, but nothing silently changes underneath them.
	recoveryFail resumeRecovery = iota

	// recoveryStartFresh begins a new conversation and answers normally. The
	// learner keeps working, but the tutor has quietly forgotten everything
	// said so far -- and will act as though the session just began.
	recoveryStartFresh

	// recoveryStartFreshAndNotify begins a new conversation and prepends a short
	// notice (see resumeNotice) so the learner knows the thread was lost.
	recoveryStartFreshAndNotify
)

// sessionGoneMarkers are the stderr phrases that positively identify "the
// conversation you asked to resume does not exist". Claude Code 2.1 prints
// "No conversation found with session ID: <uuid>"; the others are defensive
// against wording drift across versions.
var sessionGoneMarkers = []string{
	"no conversation found",
	"session not found",
	"no such session",
	"could not find session",
	"session does not exist",
	"unknown session id",
}

// decideResumeRecovery is called when `claude --resume <uuid>` exits non-zero.
//
// This happens for several different reasons, and they do NOT all deserve the
// same response:
//
//   - The session genuinely no longer exists. Claude prunes old sessions, and
//     the learner may have cleared ~/.claude. stderr typically mentions the
//     session ID not being found. Restarting is the only way forward.
//   - A transient failure: the model API was unreachable, a rate limit was hit,
//     the process was killed. The conversation is still perfectly good.
//     Restarting here THROWS AWAY a valid session for no reason.
//   - A malformed invocation on our side (bad flag, bad schema). Restarting
//     will fail identically, just twice as slowly.
//
// err is the exec error (typically *exec.ExitError); stderr is what the CLI
// printed, which is where the distinguishing detail lives.
//
// The policy is deliberately asymmetric. A missing session must be recognised
// positively from stderr before we discard anything; every other failure --
// including one we do not recognise at all -- preserves the conversation and
// surfaces the error. Getting this wrong in the permissive direction silently
// erases a learner's session; getting it wrong in the conservative direction
// shows them one error they can retry past. Those costs are not symmetric.
//
// Exit codes cannot be used to discriminate: a pruned session and an unknown
// CLI flag both exit 1.
func decideResumeRecovery(err error, stderr string) resumeRecovery {
	if err == nil {
		return recoveryFail
	}

	// We cancelled the turn ourselves, or the process was signalled. The
	// conversation is untouched and the caller already knows why.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return recoveryFail
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ProcessState != nil {
		if status, ok := exitErr.Sys().(interface{ Signaled() bool }); ok && status.Signaled() {
			return recoveryFail
		}
	}

	haystack := strings.ToLower(stderr)
	for _, marker := range sessionGoneMarkers {
		if strings.Contains(haystack, marker) {
			return recoveryStartFreshAndNotify
		}
	}

	// Anything else -- transient API failures, rate limits, our own bad flags,
	// and messages we have never seen -- keeps the conversation.
	return recoveryFail
}
