package acpbridge

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/haflettjm/llm-tutor/internal/types/lesson"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
	"github.com/haflettjm/llm-tutor/internal/types/status"
)

// ── Fake daemon ───────────────────────────────────────────────────────────────

type fakeClient struct {
	mu       sync.Mutex
	lastReq  request.Request
	queries  int
	resp     response.Response
	queryErr error

	progress status.Progress
	plans    status.Plans
	track    string
	trackErr error

	// block, when set, holds Query until released so cancellation is testable.
	block chan struct{}
}

func (f *fakeClient) Query(ctx context.Context, req request.Request) (response.Response, error) {
	f.mu.Lock()
	f.lastReq = req
	f.queries++
	block := f.block
	f.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return response.Response{}, ctx.Err()
		}
	}
	return f.resp, f.queryErr
}

func (f *fakeClient) QueryStream(ctx context.Context, req request.Request, onChunk func(string, bool) error) (response.Response, error) {
	resp, err := f.Query(ctx, req)
	if err != nil {
		return response.Response{}, err
	}
	if err := onChunk(resp.Message, false); err != nil {
		return response.Response{}, err
	}
	return resp, nil
}

func (f *fakeClient) Progress(context.Context) (status.Progress, error) { return f.progress, nil }
func (f *fakeClient) Plans(context.Context) (status.Plans, error)       { return f.plans, nil }
func (f *fakeClient) SetTrack(_ context.Context, track string) (status.Progress, error) {
	if f.trackErr != nil {
		return status.Progress{}, f.trackErr
	}
	f.track = track
	f.progress.Track = track
	return f.progress, nil
}

func (f *fakeClient) queryCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries
}

// newAgent wires an Agent to a fake daemon and captures streamed updates.
func newAgent(t *testing.T, client *fakeClient) (*Agent, *[]sdk.SessionNotification) {
	t.Helper()
	agent := New(client)
	var (
		mu      sync.Mutex
		updates []sdk.SessionNotification
	)
	agent.update = func(_ context.Context, n sdk.SessionNotification) error {
		mu.Lock()
		defer mu.Unlock()
		updates = append(updates, n)
		return nil
	}
	return agent, &updates
}

func newSession(t *testing.T, agent *Agent) sdk.SessionId {
	t.Helper()
	s, err := agent.NewSession(context.Background(), sdk.NewSessionRequest{
		Cwd: t.TempDir(), McpServers: []sdk.McpServer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s.SessionId
}

func prompt(t *testing.T, agent *Agent, session sdk.SessionId, text string) sdk.PromptResponse {
	t.Helper()
	res, err := agent.Prompt(context.Background(), sdk.PromptRequest{
		SessionId: session,
		Prompt:    []sdk.ContentBlock{sdk.TextBlock(text)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// lastText returns the text of the most recent agent message chunk.
func lastText(t *testing.T, updates *[]sdk.SessionNotification) string {
	t.Helper()
	for i := len(*updates) - 1; i >= 0; i-- {
		if c := (*updates)[i].Update.AgentMessageChunk; c != nil && c.Content.Text != nil {
			return c.Content.Text.Text
		}
	}
	t.Fatal("no agent message was streamed")
	return ""
}

// ── Free-form conversation ────────────────────────────────────────────────────

func TestPromptForwardsTextAndStreamsTutorResponse(t *testing.T) {
	client := &fakeClient{resp: response.Response{Message: "What do you predict?"}}
	agent, updates := newAgent(t, client)
	session := newSession(t, agent)

	res := prompt(t, agent, session, "Why does this loop stop?")

	if res.StopReason != sdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", res.StopReason)
	}
	if client.lastReq.Message != "Why does this loop stop?" {
		t.Errorf("forwarded message = %q", client.lastReq.Message)
	}
	if client.lastReq.SessionID != string(session) {
		t.Errorf("forwarded session = %q, want %q", client.lastReq.SessionID, session)
	}
	if got := lastText(t, updates); got != "What do you predict?" {
		t.Errorf("streamed %q", got)
	}
}

func TestNewSessionPublishesTheCommandMenu(t *testing.T) {
	agent, updates := newAgent(t, &fakeClient{})
	newSession(t, agent)

	if len(*updates) == 0 {
		t.Fatal("no update was sent on session/new")
	}
	menu := (*updates)[0].Update.AvailableCommandsUpdate
	if menu == nil {
		t.Fatalf("first update was not a command menu: %+v", (*updates)[0].Update)
	}
	if len(menu.AvailableCommands) != len(commands) {
		t.Fatalf("published %d commands, want %d", len(menu.AvailableCommands), len(commands))
	}

	byName := map[string]sdk.AvailableCommand{}
	for _, c := range menu.AvailableCommands {
		byName[c.Name] = c
	}
	for _, want := range []string{"help", "progress", "plans", "switch", "next", "end"} {
		c, ok := byName[want]
		if !ok {
			t.Errorf("command %q is missing from the menu", want)
			continue
		}
		if c.Description == "" {
			t.Errorf("command %q has no description", want)
		}
	}
	if in := byName["switch"].Input; in == nil || in.Unstructured == nil || in.Unstructured.Hint == "" {
		t.Error("switch should advertise that it takes an argument")
	}
}

func TestEmptyPromptIsRejected(t *testing.T) {
	agent, _ := newAgent(t, &fakeClient{})
	session := newSession(t, agent)
	if _, err := agent.Prompt(context.Background(), sdk.PromptRequest{
		SessionId: session, Prompt: []sdk.ContentBlock{sdk.TextBlock("   ")},
	}); err == nil {
		t.Fatal("expected an error for a blank prompt")
	}
}

func TestEmptyTutorReplyIsAnError(t *testing.T) {
	client := &fakeClient{resp: response.Response{Message: "  "}}
	agent, _ := newAgent(t, client)
	session := newSession(t, agent)
	if _, err := agent.Prompt(context.Background(), sdk.PromptRequest{
		SessionId: session, Prompt: []sdk.ContentBlock{sdk.TextBlock("hi")},
	}); err == nil {
		t.Fatal("expected an error when the tutor returns nothing")
	}
}

func TestDaemonFailureIsSurfacedInTheTranscript(t *testing.T) {
	client := &fakeClient{queryErr: errors.New("connect to tutor daemon: no such file")}
	agent, updates := newAgent(t, client)
	session := newSession(t, agent)

	if _, err := agent.Prompt(context.Background(), sdk.PromptRequest{
		SessionId: session, Prompt: []sdk.ContentBlock{sdk.TextBlock("hi")},
	}); err == nil {
		t.Fatal("expected the error to propagate")
	}
	if got := lastText(t, updates); !strings.Contains(got, "no such file") {
		t.Errorf("learner was not told why: %q", got)
	}
}

// ── Command parsing ───────────────────────────────────────────────────────────

func TestParseCommandRecognisesKnownCommands(t *testing.T) {
	cmd, args, ok := parseCommand("/switch  programming-fundamentals ")
	if !ok {
		t.Fatal("switch was not recognised")
	}
	if cmd.name != "switch" || args != "programming-fundamentals" {
		t.Fatalf("cmd = %q args = %q", cmd.name, args)
	}
}

func TestParseCommandIsCaseInsensitive(t *testing.T) {
	if cmd, _, ok := parseCommand("/PROGRESS"); !ok || cmd.name != "progress" {
		t.Fatalf("uppercase command not recognised: %v", ok)
	}
}

// A path is not a command. This is why a slash prefix alone cannot be the test.
func TestParseCommandLeavesOrdinaryQuestionsAlone(t *testing.T) {
	for _, text := range []string{
		"/tmp/cache.go is throwing a nil pointer, why?",
		"why does /etc/hosts get read first?",
		"progress",
		"",
		"/unknowncommand",
	} {
		if _, _, ok := parseCommand(text); ok {
			t.Errorf("%q was wrongly treated as a command", text)
		}
	}
}

// ── Local commands ────────────────────────────────────────────────────────────

func TestLocalCommandsDoNotSpendAModelTurn(t *testing.T) {
	client := &fakeClient{
		progress: status.Progress{Track: "programming-fundamentals", Total: 10, Demonstrated: 3},
		plans:    status.Plans{Plans: []lesson.Summary{{ID: "programming-fundamentals", Title: "Programming Fundamentals", Concepts: 10}}},
	}
	agent, _ := newAgent(t, client)
	session := newSession(t, agent)

	for _, cmd := range []string{"/help", "/progress", "/plans", "/switch programming-fundamentals"} {
		prompt(t, agent, session, cmd)
	}
	if n := client.queryCount(); n != 0 {
		t.Fatalf("local commands made %d model calls, want 0", n)
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	agent, updates := newAgent(t, &fakeClient{})
	session := newSession(t, agent)
	prompt(t, agent, session, "/help")

	got := lastText(t, updates)
	for _, c := range commands {
		if !strings.Contains(got, "/"+c.name) {
			t.Errorf("help does not mention /%s", c.name)
		}
	}
}

func TestProgressRendersPositionAndNextConcept(t *testing.T) {
	client := &fakeClient{progress: status.Progress{
		Track: "programming-fundamentals", TrackTitle: "Programming Fundamentals",
		Total: 10, Demonstrated: 3, Position: 4, Sessions: 2, ActiveSoul: "concepts-tutor",
		NextConcept: &lesson.Concept{
			ID: "PROG-004", Title: "Loops and Iteration",
			Objective: "Traces the loop variable.", Evidence: "Fills a 5-row trace table.",
		},
	}}
	agent, updates := newAgent(t, client)
	session := newSession(t, agent)
	prompt(t, agent, session, "/progress")

	got := lastText(t, updates)
	for _, want := range []string{"Programming Fundamentals", "3/10", "PROG-004", "Loops and Iteration", "4 of 10", "concepts-tutor"} {
		if !strings.Contains(got, want) {
			t.Errorf("progress output missing %q:\n%s", want, got)
		}
	}
}

func TestProgressWithNoTrackTellsYouWhatToDo(t *testing.T) {
	client := &fakeClient{progress: status.Progress{Note: "no lesson plan selected yet -- run the plans command"}}
	agent, updates := newAgent(t, client)
	session := newSession(t, agent)
	prompt(t, agent, session, "/progress")

	if got := lastText(t, updates); !strings.Contains(got, "plans command") {
		t.Errorf("no actionable guidance: %q", got)
	}
}

func TestSwitchWithoutAnArgumentListsTheChoices(t *testing.T) {
	client := &fakeClient{plans: status.Plans{Plans: []lesson.Summary{
		{ID: "programming-fundamentals", Title: "Programming Fundamentals"},
		{ID: "architecture", Title: "Architecture"},
	}}}
	agent, updates := newAgent(t, client)
	session := newSession(t, agent)
	prompt(t, agent, session, "/switch")

	got := lastText(t, updates)
	if !strings.Contains(got, "architecture") || !strings.Contains(got, "programming-fundamentals") {
		t.Errorf("switch with no argument did not list tracks:\n%s", got)
	}
	if client.track != "" {
		t.Errorf("switch with no argument changed the track to %q", client.track)
	}
}

func TestSwitchSetsTheTrack(t *testing.T) {
	client := &fakeClient{}
	agent, _ := newAgent(t, client)
	session := newSession(t, agent)
	prompt(t, agent, session, "/switch architecture")

	if client.track != "architecture" {
		t.Fatalf("track = %q, want architecture", client.track)
	}
}

func TestPlansMarksTheActiveTrack(t *testing.T) {
	client := &fakeClient{plans: status.Plans{
		Active: "architecture",
		Plans: []lesson.Summary{
			{ID: "programming-fundamentals", Title: "Programming Fundamentals"},
			{ID: "architecture", Title: "Architecture"},
		},
	}}
	agent, updates := newAgent(t, client)
	session := newSession(t, agent)
	prompt(t, agent, session, "/plans")

	got := lastText(t, updates)
	if !strings.Contains(got, "* architecture") {
		t.Errorf("active track not marked:\n%s", got)
	}
}

// ── Routed commands ───────────────────────────────────────────────────────────

func TestRoutedCommandsBecomeADirectiveForTheModel(t *testing.T) {
	client := &fakeClient{resp: response.Response{Message: "ok"}}
	agent, _ := newAgent(t, client)
	session := newSession(t, agent)

	prompt(t, agent, session, "/next")
	if !strings.Contains(client.lastReq.Message, "get_next_concept") {
		t.Errorf("/next directive = %q", client.lastReq.Message)
	}

	prompt(t, agent, session, "/end")
	if !strings.Contains(client.lastReq.Message, "end_session") {
		t.Errorf("/end directive = %q", client.lastReq.Message)
	}
}

func TestEndCarriesTheLearnersOwnNote(t *testing.T) {
	client := &fakeClient{resp: response.Response{Message: "ok"}}
	agent, _ := newAgent(t, client)
	session := newSession(t, agent)

	prompt(t, agent, session, "/end pointers finally clicked")
	if !strings.Contains(client.lastReq.Message, "pointers finally clicked") {
		t.Errorf("learner note was dropped: %q", client.lastReq.Message)
	}
}

// ── Cancellation ──────────────────────────────────────────────────────────────

func TestCancelStopsTheInFlightTurn(t *testing.T) {
	client := &fakeClient{block: make(chan struct{}), resp: response.Response{Message: "too late"}}
	agent, _ := newAgent(t, client)
	session := newSession(t, agent)

	done := make(chan sdk.PromptResponse, 1)
	go func() {
		res, _ := agent.Prompt(context.Background(), sdk.PromptRequest{
			SessionId: session, Prompt: []sdk.ContentBlock{sdk.TextBlock("a long question")},
		})
		done <- res
	}()

	// Wait until the turn is actually registered before cancelling it.
	deadline := time.After(2 * time.Second)
	for client.queryCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("query never started")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	if err := agent.Cancel(context.Background(), sdk.CancelNotification{SessionId: session}); err != nil {
		t.Fatal(err)
	}

	select {
	case res := <-done:
		if res.StopReason != sdk.StopReasonCancelled {
			t.Fatalf("stop reason = %q, want cancelled", res.StopReason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop the turn")
	}
}

func TestCancellingAnIdleSessionIsHarmless(t *testing.T) {
	agent, _ := newAgent(t, &fakeClient{})
	session := newSession(t, agent)
	if err := agent.Cancel(context.Background(), sdk.CancelNotification{SessionId: session}); err != nil {
		t.Fatal(err)
	}
}

func TestCloseSessionCancelsItsTurn(t *testing.T) {
	client := &fakeClient{block: make(chan struct{})}
	agent, _ := newAgent(t, client)
	session := newSession(t, agent)

	done := make(chan struct{})
	go func() {
		_, _ = agent.Prompt(context.Background(), sdk.PromptRequest{
			SessionId: session, Prompt: []sdk.ContentBlock{sdk.TextBlock("q")},
		})
		close(done)
	}()
	for client.queryCount() == 0 {
		time.Sleep(time.Millisecond)
	}
	if _, err := agent.CloseSession(context.Background(), sdk.CloseSessionRequest{SessionId: session}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("closing the session did not stop its turn")
	}
}
