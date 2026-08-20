package harness

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
// TODO(you): implement the policy. Returning recoveryFail for everything is the
// conservative placeholder -- it never destroys a good session, but it means a
// pruned session bricks the conversation until the learner starts a new one.
func decideResumeRecovery(err error, stderr string) resumeRecovery {
	return recoveryFail
}
