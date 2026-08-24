# Zed and Neovim Functional Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make llm-tutor genuinely usable as a daily tutor inside both Zed and Neovim: replies stream as they are generated, both editor paths are verified against the real clients, and the daemon is always there when the editor asks for it.

**Architecture:** The daemon keeps its current shape. A new optional `Streamer` interface on the harness lets `claude-code` emit incremental text; the daemon re-publishes those chunks over a new SSE route; both adapters consume that route and append to their transcript as text arrives. Nothing about learner state, soul selection, or MCP changes. Non-streaming clients keep working through the existing `POST /tutor`.

**Tech Stack:** Go 1.26, gin, `coder/acp-go-sdk` v0.13.5, claude CLI 2.1.239, Lua with `curl` via `vim.fn.jobstart`, systemd user units.

**Spec:** This plan is the spec. Source material is `notes.md` (decision records for the ACP migration and the session-start protocol) and the Status section of `README.md`.

## Global Constraints

- Go 1.26+. The module is `github.com/haflettjm/llm-tutor`.
- Never use em dashes in code, comments, commit messages, docs, or content files.
- `make check` (fmt, vet, test) must pass before every commit.
- `claude` CLI 2.1.239 is what is installed. `README.md` currently claims 2.1.234; update it when touching docs.
- The existing `POST /tutor` route must keep working unchanged throughout. Streaming is additive.
- No `ANTHROPIC_API_KEY`. The daemon shells out to the harness CLI, which uses its own credentials.
- Harnesses other than `claude-code` are out of scope. They must continue to compile and to fail with their current explicit "not yet implemented" error.

---

## Phase 0: Resolve the one real unknown

### Task 1: Spike the interaction between `--json-schema` and `stream-json`

Everything downstream depends on this answer, so it is resolved first and written down.

The daemon currently runs the CLI with `--output-format json --json-schema <responseSchema>` (`internal/app/harness/claudecode.go:141-146`). The schema forces the model to emit a JSON object with `message`, `response_type`, `hint_level` and optional `concept_id`. Under `--output-format stream-json --include-partial-messages`, the text deltas will therefore be fragments of that JSON object, not clean prose.

**Files:**
- Create: `docs/plans/streaming-spike-result.md`

- [ ] **Step 1: Capture a real event stream with the schema active**

```bash
cd /tmp && mkdir -p spike && cd spike
printf 'Ask me one short Socratic question about loops.' | \
  claude -p --verbose \
    --output-format stream-json \
    --include-partial-messages \
    --json-schema '{"type":"object","properties":{"message":{"type":"string"},"response_type":{"type":"string","enum":["question","observation","hint","explanation"]},"hint_level":{"type":"integer","minimum":0,"maximum":3},"concept_id":{"type":"string"}},"required":["message","response_type","hint_level"]}' \
  > stream.ndjson 2>stream.err
wc -l stream.ndjson && head -5 stream.ndjson
```

- [ ] **Step 2: Identify the delta event shape**

Record, verbatim, in `docs/plans/streaming-spike-result.md`:
- The `type` values that appear, one line each, with a count.
- Which event carries incremental text, and the exact JSON path to that text.
- Whether that text is raw JSON object fragments or plain prose.
- Whether the final `type:"result"` envelope is still present and still parses with the existing `parseClaudeOutput`.

- [ ] **Step 3: Record the decision**

Write one of these two verdicts into the spike doc, with the evidence:

- **Verdict A, deltas are JSON fragments.** Proceed with Task 4 as written: an incremental scanner extracts the `message` field value as it is built.
- **Verdict B, deltas are plain prose and the schema is applied only at the end.** Task 4 shrinks to passing text through unchanged. Note it and simplify Task 4 to a pass-through.

There is also a fallback if the schema turns out to be incompatible with partial messages entirely: drop `--json-schema` on streaming turns only and recover the metadata from the MCP side. The model already reports the same two fields through `append_learning_event` (`internal/app/mcp/server.go:652-653`), so `response_type` and `hint_level` are recoverable without the schema. Record this as Verdict C if it applies.

- [ ] **Step 4: Commit**

```bash
git add docs/plans/streaming-spike-result.md
git commit -m "docs: record stream-json and json-schema spike result"
```

---

## Phase 1: Harness streaming

### Task 2: Add the `Streamer` interface

**Files:**
- Modify: `internal/app/harness/harness.go`
- Test: `internal/app/harness/harness_test.go`

**Interfaces:**
- Produces: `harness.StreamChunk`, `harness.Streamer`, `harness.CanStream(Harness) (Streamer, bool)`

- [ ] **Step 1: Write the failing test**

```go
func TestCanStreamReportsFalseForNonStreamingHarness(t *testing.T) {
	h := &codex{Base: Base{promptFile: "CODEX.md"}}
	if _, ok := CanStream(h); ok {
		t.Fatal("codex must not report streaming support")
	}
}

func TestCanStreamReportsTrueForClaudeCode(t *testing.T) {
	h := newClaudeCode(t.TempDir())
	if _, ok := CanStream(h); !ok {
		t.Fatal("claude-code must report streaming support")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/harness/ -run TestCanStream -v`
Expected: FAIL, `undefined: CanStream`

- [ ] **Step 3: Add the interface**

```go
// StreamChunk is one incremental piece of a tutor reply.
type StreamChunk struct {
	// Text is new message text to append to what the learner has already seen.
	Text string
}

// Streamer is implemented by harnesses whose CLI can emit output incrementally.
// A harness that does not implement it still works; the caller falls back to
// Query and delivers the reply whole.
type Streamer interface {
	// StreamQuery runs one turn, calling emit for each incremental piece as it
	// arrives, and returns the same complete Response that Query would.
	// An error from emit aborts the turn and is returned.
	StreamQuery(ctx context.Context, req request.Request, emit func(StreamChunk) error) (response.Response, error)
}

// CanStream reports whether h can stream, and returns its Streamer view.
func CanStream(h Harness) (Streamer, bool) {
	s, ok := h.(Streamer)
	return s, ok
}
```

The second test will fail until Task 5 lands. Mark it `t.Skip("implemented in Task 5")` for now and remove the skip there.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/app/harness/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/harness/harness.go internal/app/harness/harness_test.go
git commit -m "harness: add optional Streamer interface"
```

---

### Task 3: Decode the stream-json NDJSON envelope

**Files:**
- Create: `internal/app/harness/streamjson.go`
- Test: `internal/app/harness/streamjson_test.go`

**Interfaces:**
- Consumes: the event shape recorded in Task 1.
- Produces: `scanStreamJSON(r io.Reader, onText func(string) error) (finalEnvelope []byte, err error)`

- [ ] **Step 1: Write the failing test**

Use the real captured events from Task 1. Substitute the exact field path the spike recorded if it differs.

```go
func TestScanStreamJSONCollectsTextAndFinalEnvelope(t *testing.T) {
	const input = `{"type":"system","subtype":"init"}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello "}}}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"world"}}}
{"type":"result","subtype":"success","result":{"message":"Hello world","response_type":"question","hint_level":0}}
`
	var got strings.Builder
	final, err := scanStreamJSON(strings.NewReader(input), func(s string) error {
		got.WriteString(s)
		return nil
	})
	if err != nil {
		t.Fatalf("scanStreamJSON: %v", err)
	}
	if got.String() != "Hello world" {
		t.Errorf("text = %q, want %q", got.String(), "Hello world")
	}
	if !bytes.Contains(final, []byte(`"type":"result"`)) {
		t.Errorf("final envelope = %q, want the result envelope", final)
	}
}

// A single NDJSON line can exceed bufio.Scanner's default 64KB limit when a
// diff is echoed back, which would silently truncate the turn.
func TestScanStreamJSONHandlesVeryLongLines(t *testing.T) {
	long := strings.Repeat("x", 200_000)
	input := `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"` + long + `"}}}` + "\n" +
		`{"type":"result","subtype":"success","result":{"message":"ok","response_type":"question","hint_level":0}}` + "\n"

	var n int
	if _, err := scanStreamJSON(strings.NewReader(input), func(s string) error {
		n += len(s)
		return nil
	}); err != nil {
		t.Fatalf("scanStreamJSON: %v", err)
	}
	if n != len(long) {
		t.Errorf("got %d bytes of text, want %d", n, len(long))
	}
}

func TestScanStreamJSONPropagatesEmitError(t *testing.T) {
	const input = `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"a"}}}` + "\n"
	want := errors.New("client gone")
	_, err := scanStreamJSON(strings.NewReader(input), func(string) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// Unknown event types must not abort the turn. The CLI adds new ones freely.
func TestScanStreamJSONIgnoresUnknownEvents(t *testing.T) {
	const input = `{"type":"some_future_event","payload":{"nested":true}}
{"type":"result","subtype":"success","result":{"message":"ok","response_type":"question","hint_level":0}}
`
	if _, err := scanStreamJSON(strings.NewReader(input), func(string) error { return nil }); err != nil {
		t.Fatalf("unknown event aborted the scan: %v", err)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/harness/ -run TestScanStreamJSON -v`
Expected: FAIL, `undefined: scanStreamJSON`

- [ ] **Step 3: Implement**

```go
package harness

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// maxStreamLine bounds a single NDJSON line. A turn that echoes a large diff
// produces lines far past bufio.Scanner's 64KB default, and the default fails
// the whole turn with a bare "token too long".
const maxStreamLine = 8 << 20

// scanStreamJSON reads the CLI's stream-json output, calling onText for each
// incremental text delta, and returns the final result envelope for the
// existing parseClaudeOutput to handle.
//
// Unknown event types are skipped rather than treated as errors: the CLI adds
// event types between releases, and a tutor turn must not die because of one.
func scanStreamJSON(r io.Reader, onText func(string) error) ([]byte, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxStreamLine)

	var final []byte
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}

		var probe struct {
			Type  string `json:"type"`
			Event struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			} `json:"event"`
		}
		if json.Unmarshal(line, &probe) != nil {
			continue // not JSON, not ours
		}

		switch probe.Type {
		case "result":
			final = append(final[:0], line...)
		case "stream_event":
			if probe.Event.Delta.Text == "" {
				continue
			}
			if err := onText(probe.Event.Delta.Text); err != nil {
				return nil, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read stream-json: %w", err)
	}
	return final, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/app/harness/ -run TestScanStreamJSON -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/harness/streamjson.go internal/app/harness/streamjson_test.go
git commit -m "harness: decode stream-json NDJSON events"
```

---

### Task 4: Extract the growing `message` field from partial JSON

Skip this task entirely if Task 1 recorded Verdict B or Verdict C. Under Verdict A the deltas are fragments of a JSON object, so raw deltas cannot be shown to the learner: they would see `{"message":"What happ`.

**Files:**
- Create: `internal/app/harness/messagescan.go`
- Test: `internal/app/harness/messagescan_test.go`

**Interfaces:**
- Produces: `type messageScanner struct{}` with `func (s *messageScanner) Feed(fragment string) string`

- [ ] **Step 1: Write the failing test**

```go
func TestMessageScannerExtractsAcrossFragmentBoundaries(t *testing.T) {
	var s messageScanner
	var got strings.Builder
	// The split points are deliberately hostile: mid-key, mid-value, mid-escape.
	for _, frag := range []string{`{"mess`, `age":"What ha`, `ppens if i st`, `arts at 1?","response_type":"question"}`} {
		got.WriteString(s.Feed(frag))
	}
	if got.String() != "What happens if i starts at 1?" {
		t.Errorf("got %q", got.String())
	}
}

func TestMessageScannerDecodesEscapes(t *testing.T) {
	var s messageScanner
	got := s.Feed(`{"message":"line one\nline \"two\"","hint_level":0}`)
	want := "line one\nline \"two\""
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The model may emit response_type before message. Keying off byte position
// rather than the field name would silently stream the wrong field.
func TestMessageScannerHandlesMessageNotFirst(t *testing.T) {
	var s messageScanner
	got := s.Feed(`{"response_type":"hint","message":"try tracing it"}`)
	if got != "try tracing it" {
		t.Errorf("got %q", got)
	}
}

func TestMessageScannerStopsAtEndOfValue(t *testing.T) {
	var s messageScanner
	first := s.Feed(`{"message":"done","concept_id":"PROG-004"}`)
	second := s.Feed(`trailing garbage`)
	if first != "done" || second != "" {
		t.Errorf("first=%q second=%q", first, second)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/harness/ -run TestMessageScanner -v`
Expected: FAIL, `undefined: messageScanner`

- [ ] **Step 3: Implement**

```go
package harness

import "strings"

// messageScanner pulls the value of the top-level "message" key out of a JSON
// object that arrives in fragments, decoding escapes as it goes.
//
// It exists because --json-schema makes the model emit a JSON object, so the
// raw stream deltas are JSON source. Showing those to a learner would render
// `{"message":"What happ` in the transcript.
type messageScanner struct {
	pre   strings.Builder // buffered bytes before the value starts
	inVal bool
	done  bool
	esc   bool
	uni   []byte // collecting a \uXXXX escape
}

const messageKey = `"message":"`

// Feed consumes the next fragment and returns any newly decoded message text.
func (s *messageScanner) Feed(fragment string) string {
	if s.done {
		return ""
	}
	var out strings.Builder

	for i := 0; i < len(fragment); i++ {
		c := fragment[i]

		if !s.inVal {
			s.pre.WriteByte(c)
			// Keep only enough tail to match the key across a boundary.
			if s.pre.Len() > len(messageKey)*4 {
				t := s.pre.String()
				s.pre.Reset()
				s.pre.WriteString(t[len(t)-len(messageKey):])
			}
			if strings.HasSuffix(s.pre.String(), messageKey) {
				s.inVal = true
				s.pre.Reset()
			}
			continue
		}

		switch {
		case len(s.uni) > 0:
			s.uni = append(s.uni, c)
			if len(s.uni) == 4 {
				var r rune
				for _, h := range s.uni {
					r = r*16 + rune(hexVal(h))
				}
				out.WriteRune(r)
				s.uni = nil
			}
		case s.esc:
			s.esc = false
			switch c {
			case 'n':
				out.WriteByte('\n')
			case 't':
				out.WriteByte('\t')
			case 'r':
				out.WriteByte('\r')
			case 'u':
				s.uni = make([]byte, 0, 4)
			default:
				out.WriteByte(c) // covers \" \\ \/
			}
		case c == '\\':
			s.esc = true
		case c == '"':
			s.done = true
			return out.String()
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/app/harness/ -run TestMessageScanner -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/harness/messagescan.go internal/app/harness/messagescan_test.go
git commit -m "harness: extract message field from partial JSON deltas"
```

---

### Task 5: Implement `claudeCode.StreamQuery`

**Files:**
- Modify: `internal/app/harness/claudecode.go`
- Test: `internal/app/harness/claudecode_stream_test.go`

**Interfaces:**
- Consumes: `scanStreamJSON`, `messageScanner`, existing `parseClaudeOutput`, `decideResumeRecovery`, `claudeError`.
- Produces: `func (c *claudeCode) StreamQuery(ctx context.Context, req request.Request, emit func(StreamChunk) error) (response.Response, error)`

The resume-recovery behaviour of `Query` must be preserved exactly. A pruned session during a streaming turn has to restart and prepend `resumeNotice` the same way, and any text already emitted before the failure must not be double counted.

- [ ] **Step 1: Write the failing test**

Drive it through a fake `claude` on PATH, the same way `claudecode_test.go` already does. Reuse that helper.

```go
func TestStreamQueryEmitsIncrementalTextAndReturnsFullResponse(t *testing.T) {
	dir := t.TempDir()
	withFakeClaude(t, `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"{\"message\":\"What "}}}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"changes?\",\"response_type\":\"question\",\"hint_level\":0}"}}}
{"type":"result","subtype":"success","result":{"message":"What changes?","response_type":"question","hint_level":0}}`)

	c := newClaudeCode(dir)
	var got strings.Builder
	resp, err := c.StreamQuery(context.Background(),
		request.Request{Message: "hi", SessionID: "s1"},
		func(ch StreamChunk) error { got.WriteString(ch.Text); return nil })
	if err != nil {
		t.Fatalf("StreamQuery: %v", err)
	}
	if got.String() != "What changes?" {
		t.Errorf("streamed %q, want %q", got.String(), "What changes?")
	}
	if resp.Message != "What changes?" || resp.ResponseType != "question" {
		t.Errorf("resp = %+v", resp)
	}
}

// A cancelled context must stop the turn rather than run the model to completion.
func TestStreamQueryStopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	withSlowFakeClaude(t)
	c := newClaudeCode(dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.StreamQuery(ctx, request.Request{Message: "hi", SessionID: "s1"},
		func(StreamChunk) error { return nil })
	if err == nil {
		t.Fatal("want an error from a cancelled context")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/harness/ -run TestStreamQuery -v`
Expected: FAIL, `c.StreamQuery undefined`

- [ ] **Step 3: Implement**

```go
// streamArgs are the flags that turn one turn into an incremental one.
// --verbose is required by the CLI alongside stream-json in print mode.
func streamArgs() []string {
	return []string{
		"-p", "--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--json-schema", responseSchema,
		"--allowedTools", allowedTools,
	}
}

func (c *claudeCode) StreamQuery(ctx context.Context, req request.Request, emit func(StreamChunk) error) (response.Response, error) {
	prompt := buildPrompt(req)

	args, resuming := c.sessionArgs(req.SessionID)
	resp, emitted, out, stderr, err := c.runStream(ctx, args, prompt, emit)
	if err == nil {
		return resp, nil
	}
	if !resuming {
		return response.Response{}, claudeError("claude stream query", err, out, stderr)
	}

	switch decideResumeRecovery(err, stderr+"\n"+envelopeDetail(out)) {
	case recoveryStartFresh, recoveryStartFreshAndNotify:
		notify := decideResumeRecovery(err, stderr+"\n"+envelopeDetail(out)) == recoveryStartFreshAndNotify

		// Anything already shown to the learner belonged to the turn that
		// died. Tell the adapter to discard it rather than let the retry
		// append onto a half-sentence.
		if emitted && emit != nil {
			if err := emit(StreamChunk{Reset: true}); err != nil {
				return response.Response{}, err
			}
		}

		c.forgetSession(req.SessionID)
		freshArgs, _ := c.sessionArgs(req.SessionID)
		resp, _, out, stderr, err = c.runStream(ctx, freshArgs, prompt, emit)
		if err != nil {
			return response.Response{}, claudeError("claude stream query after restarting session", err, out, stderr)
		}
		if notify {
			resp.Message = resumeNotice + resp.Message
		}
		return resp, nil
	}
	return response.Response{}, claudeError("claude stream query", err, out, stderr)
}

// runStream invokes the CLI and pumps its stdout through the scanners while it
// runs. stdout is NOT buffered whole: the point of the exercise is that text
// reaches the learner before the process exits.
func (c *claudeCode) runStream(ctx context.Context, sessionArgs []string, prompt string, emit func(StreamChunk) error) (resp response.Response, emitted bool, out []byte, stderr string, err error) {
	args := append(streamArgs(), sessionArgs...)

	var errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = c.dataDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return resp, false, nil, errBuf.String(), err
	}
	if err := cmd.Start(); err != nil {
		return resp, false, nil, errBuf.String(), err
	}

	var scan messageScanner
	final, scanErr := scanStreamJSON(pipe, func(raw string) error {
		text := scan.Feed(raw)
		if text == "" {
			return nil
		}
		emitted = true
		if emit == nil {
			return nil
		}
		return emit(StreamChunk{Text: text})
	})

	waitErr := cmd.Wait()
	if scanErr != nil {
		return resp, emitted, final, errBuf.String(), scanErr
	}
	if waitErr != nil {
		return resp, emitted, final, errBuf.String(), waitErr
	}
	resp, err = parseClaudeOutput(final)
	return resp, emitted, final, errBuf.String(), err
}
```

Add the `Reset` field to `StreamChunk` in `internal/app/harness/harness.go`:

```go
type StreamChunk struct {
	Text string
	// Reset tells the consumer to discard everything shown so far for this
	// turn. Set when a resume failed and the turn restarted from scratch.
	Reset bool
}
```

- [ ] **Step 4: Run tests, and remove the Task 2 skip**

Run: `go test ./internal/app/harness/ -v`
Expected: PASS, including `TestCanStreamReportsTrueForClaudeCode` with its `t.Skip` removed.

- [ ] **Step 5: Commit**

```bash
git add internal/app/harness/
git commit -m "harness: stream claude-code turns incrementally"
```

---

## Phase 2: Daemon

### Task 6: `tutor.Service` gains `HandleStream`

**Files:**
- Modify: `internal/app/tutor/tutor.go`
- Test: `internal/app/tutor/tutor_stream_test.go`

**Interfaces:**
- Produces: `HandleStream(ctx context.Context, req request.Request, emit func(harness.StreamChunk) error) (response.Response, error)` on both `Service` and `*Tutor`.

The system prompt recomposition that happens before and after a turn must happen for streaming turns too, otherwise a `set_soul` mid-turn silently stops taking effect.

- [ ] **Step 1: Write the failing test**

```go
func TestHandleStreamFallsBackToQueryForNonStreamingHarness(t *testing.T) {
	tut := newTestTutor(t, &fakeHarness{reply: "whole reply"}) // does not implement Streamer
	var got strings.Builder
	resp, err := tut.HandleStream(context.Background(),
		request.Request{Message: "hi", SessionID: "s"},
		func(ch harness.StreamChunk) error { got.WriteString(ch.Text); return nil })
	if err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	// The whole reply arrives as a single chunk, so every client can use one path.
	if got.String() != "whole reply" || resp.Message != "whole reply" {
		t.Errorf("got %q, resp %+v", got.String(), resp)
	}
}

func TestHandleStreamRecomposesSystemPromptAfterTurn(t *testing.T) {
	h := &fakeStreamingHarness{chunks: []string{"a", "b"}, reply: "ab"}
	tut := newTestTutor(t, h)
	if _, err := tut.HandleStream(context.Background(),
		request.Request{Message: "hi", SessionID: "s"},
		func(harness.StreamChunk) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if h.promptWrites < 1 {
		t.Error("system prompt was never recomposed for a streaming turn")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/tutor/ -run TestHandleStream -v`
Expected: FAIL, `tut.HandleStream undefined`

- [ ] **Step 3: Implement**

Add to the `Service` interface and implement on `*Tutor`, reusing whatever pre-turn and post-turn prompt sync `Handle` already performs. A harness with no `Streamer` calls `Query` and emits the whole message as one chunk, so callers never need two code paths.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/app/tutor/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/tutor/
git commit -m "tutor: add HandleStream with whole-reply fallback"
```

---

### Task 7: SSE route `POST /tutor/stream`

**Files:**
- Modify: `internal/app/api/api.go`
- Test: `internal/app/api/api_stream_test.go`

**Interfaces:**
- Produces: route `POST /tutor/stream`, content type `text/event-stream`, events `chunk`, `reset`, `done`, `error`.

Wire format, which both adapters and the tests depend on:

```
event: chunk
data: {"text":"What happens "}

event: reset
data: {}

event: done
data: {"message":"What happens if i starts at 1?","response_type":"question","hint_level":0}

event: error
data: {"error":"claude stream query: ..."}
```

- [ ] **Step 1: Write the failing test**

```go
func TestTutorStreamEmitsChunksThenDone(t *testing.T) {
	r := Router(Deps{ /* same fixture the existing api_test.go builds */ })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/tutor/stream",
		strings.NewReader(`{"message":"hi","session_id":"s"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}
	body := w.Body.String()
	for _, want := range []string{"event: chunk", `"text":"What `, "event: done", `"response_type":"question"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
}

// A mid-turn failure must reach the client as an error event, not as a
// truncated success. A silently short reply is worse than a visible failure.
func TestTutorStreamReportsMidTurnFailureAsErrorEvent(t *testing.T) {
	r := Router(Deps{ /* fixture whose tutor fails after one chunk */ })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/tutor/stream",
		strings.NewReader(`{"message":"hi","session_id":"s"}`))
	r.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "event: error") {
		t.Errorf("no error event in:\n%s", w.Body.String())
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/api/ -run TestTutorStream -v`
Expected: FAIL, 404

- [ ] **Step 3: Implement**

```go
func (d Deps) handleTutorStream(c *gin.Context) {
	var req request.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, status.Error{Error: err.Error()})
		return
	}

	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	send := func(event string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		c.Writer.Flush() // without this the whole exercise is pointless
		return nil
	}

	resp, err := d.Tutor.HandleStream(c.Request.Context(), req, func(ch harness.StreamChunk) error {
		if ch.Reset {
			return send("reset", struct{}{})
		}
		return send("chunk", map[string]string{"text": ch.Text})
	})
	if err != nil {
		d.Log.Error("tutor stream", zap.Error(err))
		_ = send("error", status.Error{Error: err.Error()})
		return
	}
	_ = send("done", resp)
}
```

Register it next to the existing route:

```go
r.POST("/tutor", d.handleTutor)
r.POST("/tutor/stream", d.handleTutorStream)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/app/api/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/api/
git commit -m "api: add SSE streaming route for tutor turns"
```

---

## Phase 3: Zed

### Task 8: `Client.QueryStream` over SSE

**Files:**
- Modify: `internal/app/acp/client.go`
- Test: `internal/app/acp/client_stream_test.go`

**Interfaces:**
- Produces: `QueryStream(ctx context.Context, req request.Request, onChunk func(text string, reset bool) error) (response.Response, error)` added to the `Client` interface.

Every existing fake implementing `Client` in `agent_test.go` must gain the method. Check them before starting.

- [ ] **Step 1: Write the failing test**

```go
func TestQueryStreamParsesSSEEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: chunk\ndata: {\"text\":\"one \"}\n\n")
		io.WriteString(w, "event: chunk\ndata: {\"text\":\"two\"}\n\n")
		io.WriteString(w, "event: done\ndata: {\"message\":\"one two\",\"response_type\":\"question\",\"hint_level\":0}\n\n")
	}))
	defer srv.Close()

	c := newTestClientPointedAt(t, srv.URL)
	var got strings.Builder
	resp, err := c.QueryStream(context.Background(), request.Request{Message: "hi"},
		func(text string, reset bool) error { got.WriteString(text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "one two" || resp.Message != "one two" {
		t.Errorf("got %q resp %+v", got.String(), resp)
	}
}

func TestQueryStreamSurfacesErrorEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: error\ndata: {\"error\":\"model refused\"}\n\n")
	}))
	defer srv.Close()

	c := newTestClientPointedAt(t, srv.URL)
	_, err := c.QueryStream(context.Background(), request.Request{Message: "hi"},
		func(string, bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "model refused") {
		t.Fatalf("err = %v, want it to carry the daemon's reason", err)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/acp/ -run TestQueryStream -v`
Expected: FAIL, `QueryStream undefined`

- [ ] **Step 3: Implement**

Parse the SSE frames with `bufio.Scanner`, splitting on blank lines, tracking the current `event:` and `data:`. Do not buffer the whole body. Set the same `maxStreamLine` bound as Task 3.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/app/acp/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/acp/
git commit -m "acp: consume the daemon SSE stream"
```

---

### Task 9: `Agent.Prompt` streams into the transcript

**Files:**
- Modify: `internal/app/acp/agent.go:87-139`
- Test: `internal/app/acp/agent_test.go`

Local commands (`/progress`, `/plans`, `/switch`) must keep answering from local state with no model turn and no streaming.

- [ ] **Step 1: Write the failing test**

```go
func TestPromptStreamsChunksAsSeparateUpdates(t *testing.T) {
	agent, updates := newAgent(t, &fakeStreamClient{chunks: []string{"What ", "changes?"}})
	if _, err := agent.Prompt(context.Background(), promptFor("s", "hi")); err != nil {
		t.Fatal(err)
	}
	if len(updates) < 2 {
		t.Fatalf("got %d updates, want one per chunk", len(updates))
	}
	if joinAgentText(updates) != "What changes?" {
		t.Errorf("transcript = %q", joinAgentText(updates))
	}
}

// A reset means the earlier turn died and restarted. The transcript must not
// end up with the abandoned half-sentence followed by the retry.
func TestPromptHandlesResetChunk(t *testing.T) {
	agent, updates := newAgent(t, &fakeStreamClient{chunks: []string{"aban", "RESET", "the real answer"}})
	if _, err := agent.Prompt(context.Background(), promptFor("s", "hi")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(joinAgentText(updates), "aban") {
		t.Errorf("abandoned text survived the reset: %q", joinAgentText(updates))
	}
}

func TestPromptLocalCommandsDoNotStream(t *testing.T) {
	c := &fakeStreamClient{}
	agent, _ := newAgent(t, c)
	if _, err := agent.Prompt(context.Background(), promptFor("s", "/progress")); err != nil {
		t.Fatal(err)
	}
	if c.queryStreamCalls != 0 {
		t.Error("/progress spent a model turn")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/acp/ -run TestPrompt -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Change `respond` to stream. Keep the existing guarantees: an empty reply is still an error, a failure is still surfaced into the transcript, and cancellation still returns `StopReasonCancelled`.

For `reset`, send `sdk.UpdateAgentMessageText` with the accumulated-so-far text cleared. Confirm against the ACP SDK whether `UpdateAgentMessageText` appends or replaces in the client; if it appends, a reset needs the adapter to buffer and re-send rather than emit a correction. Record the answer in a comment.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/app/acp/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/acp/
git commit -m "acp: stream tutor replies into the transcript"
```

---

### Task 10: Zed verification gate

This is stage 3 from `notes.md`, never met. It needs a human at the keyboard, so it cannot be done by an agent.

**Files:**
- Modify: `README.md` (the "Verification status" note)
- Modify: `notes.md`

- [ ] **Step 1: Build and install**

```bash
make install
which knumble-tutor knumble-acp
```

- [ ] **Step 2: Register in Zed**

Put this in Zed `settings.json`, with a real absolute path. Zed 1.14 rejects the bare `{"command": ...}` form and does not expand a tilde.

```json
{
  "agent_servers": {
    "knumble-tutor": {
      "type": "custom",
      "command": "/home/fivek777/.local/bin/knumble-acp",
      "args": [],
      "env": {}
    }
  }
}
```

- [ ] **Step 3: Run the gate**

Start the daemon, then in Zed's agent panel open a new thread with `knumble-tutor` and confirm each of these, writing down what actually happened:

1. The agent appears in the new-thread menu.
2. `/start` appears in the slash command menu with its description.
3. `/start` produces the first-session orientation, not a lesson.
4. A free-form question streams in visibly rather than appearing whole.
5. `/progress` answers instantly with no model turn.
6. Cancelling mid-turn actually stops it.
7. A second turn retains context from the first.

- [ ] **Step 4: Record the result**

Replace the "Verification status" paragraph in `README.md` with what was observed. If slash commands do not render, note the documented fallback: they still work as plain text without autocomplete.

- [ ] **Step 5: Commit**

```bash
git add README.md notes.md
git commit -m "docs: record Zed agent panel verification"
```

---

## Phase 4: Neovim

### Task 11: Stream into the Neovim buffer

**Files:**
- Modify: `plugin/nvim/lua/llm-tutor/init.lua:129-200`

The plugin shells out to `curl` through `vim.fn.jobstart` and decodes one JSON body on exit. Streaming needs `curl -N`, incremental `on_stdout`, and a buffer that is appended to rather than created at the end.

- [ ] **Step 1: Add an SSE request helper**

```lua
-- ── SSE request over Unix socket ──────────────────────────────────────────────
-- curl -N disables buffering, without which the whole point is lost.
local function request_stream(path, body, on_chunk, on_done, on_error)
  local cmd = {
    "curl", "-sfN", "--unix-socket", cfg.socket,
    "-X", "POST",
    "-H", "Content-Type: application/json",
    "-d", vim.fn.json_encode(body),
    "http://localhost" .. path,
  }

  local event, pending = nil, nil
  vim.fn.jobstart(cmd, {
    stdout_buffered = false,
    on_stdout = function(_, lines)
      for _, line in ipairs(lines) do
        if line:match("^event: ") then
          event = line:sub(8)
        elseif line:match("^data: ") then
          pending = line:sub(7)
        elseif line == "" and event and pending then
          local ok, payload = pcall(vim.fn.json_decode, pending)
          if ok then
            if event == "chunk" then on_chunk(payload.text)
            elseif event == "reset" then on_chunk(nil, true)
            elseif event == "done" then on_done(payload)
            elseif event == "error" then on_error(payload.error) end
          end
          event, pending = nil, nil
        end
      end
    end,
    on_exit = function(_, code)
      if code ~= 0 then
        on_error(string.format("curl exited %d, is knumble-tutor running?", code))
      end
    end,
  })
end
```

- [ ] **Step 2: Open the response buffer before the first chunk**

Split `show_response` into `open_response_buffer()` returning a handle, `append_to_response(handle, text)`, and `finalize_response(handle, resp)` which writes the `type_label` and `[level N/3]` header once the `done` event carries `response_type` and `hint_level`.

Show a placeholder header while streaming, since the real one is not known until the end.

- [ ] **Step 3: Point `:LlmAsk` and `:LlmAskDiff` at the stream**

Replace the `request("POST", "/tutor", ...)` call at line 194 with `request_stream("/tutor/stream", ...)`. Leave `/progress`, `/plans`, `/track` and `/health` on the existing non-streaming helper.

- [ ] **Step 4: Verify by hand**

```bash
make run   # in one terminal
nvim       # in another
```

Confirm: `:LlmStart` orients, `:LlmAsk` streams text in visibly, the header with the hint level appears when it completes, `:LlmProgress` is instant, and `:LlmHealth` still reports the harness.

- [ ] **Step 5: Commit**

```bash
git add plugin/nvim/lua/llm-tutor/init.lua
git commit -m "nvim: stream tutor replies into the response buffer"
```

---

### Task 12: Neovim ACP client verification gate

Stage 2 from `notes.md`, never met. Needs a human. The native plugin from Task 11 is the primary Neovim path; this gate covers the ACP path through codecompanion.nvim.

- [ ] **Step 1: Configure codecompanion.nvim to point at `knumble-acp`**
- [ ] **Step 2: Confirm the command menu renders and a turn streams**
- [ ] **Step 3: Decide and record whether the ACP path is worth supporting for Neovim at all, given the native plugin exists**

If the native plugin is better, say so in `README.md` and describe ACP as the Zed path. That is a legitimate outcome of this gate, not a failure.

- [ ] **Step 4: Commit**

```bash
git add README.md notes.md
git commit -m "docs: record Neovim ACP client verification"
```

---

## Phase 5: Always available

### Task 13: systemd user service

Stage 4 from `notes.md`, untouched. Right now the learner must remember to start the daemon, and the editor path fails confusingly when they have not.

**Files:**
- Create: `packaging/systemd/knumble-tutor.service`
- Modify: `Makefile`
- Modify: `README.md`

- [ ] **Step 1: Write the unit**

```ini
[Unit]
Description=Knumble tutor daemon
Documentation=https://github.com/haflettjm/llm-tutor
After=network.target

[Service]
Type=simple
ExecStart=%h/.local/bin/knumble-tutor
Restart=on-failure
RestartSec=2
# The socket is recreated on start; a stale one from a hard kill blocks bind.
ExecStartPre=-/bin/rm -f /tmp/llm-tutor.sock

[Install]
WantedBy=default.target
```

- [ ] **Step 2: Add a Makefile target**

```make
install-service: install
	install -Dm644 packaging/systemd/knumble-tutor.service \
		$(HOME)/.config/systemd/user/knumble-tutor.service
	systemctl --user daemon-reload
	systemctl --user enable --now knumble-tutor.service
	systemctl --user status --no-pager knumble-tutor.service
```

- [ ] **Step 3: Verify it survives a restart**

```bash
make install-service
systemctl --user restart knumble-tutor
curl -sf --unix-socket /tmp/llm-tutor.sock http://localhost/health | head
```

Expected: the health JSON, naming the harness and the active soul.

- [ ] **Step 4: Document it in README under Install**

- [ ] **Step 5: Commit**

```bash
git add packaging/ Makefile README.md
git commit -m "packaging: add systemd user service for the daemon"
```

---

### Task 14: Adapter reconnect

**Files:**
- Modify: `internal/app/acp/client.go`
- Test: `internal/app/acp/client_test.go`

When the daemon is restarting, the adapter currently fails the turn with a raw dial error. With a service supervising the daemon, a brief outage is normal and recoverable.

- [ ] **Step 1: Write the failing test**

```go
func TestClientRetriesWhileDaemonIsRestarting(t *testing.T) {
	var attempts int32
	// Fails the first two dials, then succeeds.
	c := newTestClientWithDialer(t, func() (net.Conn, error) {
		if atomic.AddInt32(&attempts, 1) <= 2 {
			return nil, errors.New("connection refused")
		}
		return dialTestServer(t)
	})
	if _, err := c.Progress(context.Background()); err != nil {
		t.Fatalf("Progress: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

// A daemon that is genuinely not running must still fail promptly with a
// message naming the socket, not hang.
func TestClientGivesUpWithAClearError(t *testing.T) {
	c := NewHTTPClient("/tmp/definitely-not-a-socket.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := c.Progress(ctx)
	if err == nil || !strings.Contains(err.Error(), "definitely-not-a-socket.sock") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/app/acp/ -run TestClient -v`

- [ ] **Step 3: Implement**

Retry dial failures only, three attempts, 200ms apart. Never retry a request that already reached the daemon: a `/tutor` turn that failed downstream must not be silently run twice and billed twice.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/app/acp/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/app/acp/
git commit -m "acp: retry dialling while the daemon restarts"
```

---

## Phase 6: Content

### Tasks 15 to 19: Concept entries for the five empty tracks

Five of six lesson plans parse to zero concepts, so `/progress` correctly reports "this track has no concept-level entries yet". This is markdown authoring against a parser that already works, one task per track, all independent of each other and of everything above.

Tracks: `algorithms-and-data-structures`, `architecture`, `backend-engineering`, `devops-and-infrastructure`, `systems-and-concurrency`.

**Files, per track:**
- Modify: `internal/app/content/lesson-plans/<track>.md`
- Test: `internal/types/lesson/lesson_test.go`

Each concept follows the format the parser reads, and every field is load-bearing. `Evidence` is what the tutor must observe before marking the concept demonstrated, and `Soul` overrides the plan's soul-mapping table.

```markdown
### ALGO-001: Big-O as a Shape, Not a Number
- **Objective:** Describes how work grows with input size, without counting operations.
- **Diagnostic:** "This loop runs 3n + 7 times. What happens when n doubles?"
- **Exercise:** Sketch the growth of a nested loop over the same list.
- **Misconception:** Big-O measures how long something takes in seconds.
- **Evidence:** Predicts the effect of doubling n on two different loops correctly.
- **Transfer:** Spots the quadratic in an unfamiliar function without running it.
- **Prerequisites:**
- **Soul:** concepts-tutor
```

- [ ] **Step 1: Write the failing test for the track**

```go
func TestAlgorithmsTrackHasOrderedConcepts(t *testing.T) {
	lib := loadShippedPlans(t)
	plan, err := lib.Plan("algorithms-and-data-structures")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Order()) < 8 {
		t.Fatalf("got %d concepts, want at least 8", len(plan.Order()))
	}
	// A concept must never be ordered before something it depends on.
	seen := map[string]bool{}
	for _, id := range plan.Order() {
		for _, pre := range plan.Concepts[id].Prerequisites {
			if !seen[pre] {
				t.Errorf("%s is ordered before its prerequisite %s", id, pre)
			}
		}
		seen[id] = true
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/types/lesson/ -run TestAlgorithmsTrack -v`
Expected: FAIL, got 0 concepts

- [ ] **Step 3: Write 8 to 12 concepts into the track file**

- [ ] **Step 4: Run tests**

Run: `go test ./internal/types/lesson/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/content/lesson-plans/algorithms-and-data-structures.md internal/types/lesson/lesson_test.go
git commit -m "content: add concept entries for the algorithms track"
```

Repeat for the remaining four tracks, one commit each.

---

## Phase 7: Close out

### Task 20: Update README and notes

**Files:**
- Modify: `README.md`
- Modify: `notes.md`

- [ ] **Step 1: Rewrite the Status section against what is actually true**

The current Status section lists streaming, the five empty tracks, and the editor gates as open. Correct each one, and update the claimed CLI version from 2.1.234 to whatever was verified.

- [ ] **Step 2: Document `POST /tutor/stream` in the Daemon API table**

- [ ] **Step 3: Add a notes.md decision record for the streaming design**

Record what the Task 1 spike found, why the `messageScanner` exists, and the reset-on-resume-failure behaviour. That last one is the least obvious thing in the codebase and the easiest to break later.

- [ ] **Step 4: Commit**

```bash
git add README.md notes.md
git commit -m "docs: bring status in line with the streaming work"
```

---

## Out of scope

Stated so nobody quietly adds them:

- `opencode`, `codex` and `hermes` harnesses. They stay explicit stubs. Their `Start` does a `LookPath` and succeeds, which means a learner can configure one, watch the daemon start cleanly, and only discover the failure on their first turn. Failing fast in `Start` is a three-line fix worth its own issue, but it is not in this plan.
- Replacing the native Neovim plugin with the ACP path. Task 12 decides that on evidence.
- Any change to MENTOR.md teaching behaviour.
