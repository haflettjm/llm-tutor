package tutor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/lesson"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

// ── Fakes ─────────────────────────────────────────────────────────────────────

type fakeHarness struct {
	writes   []string // every system prompt written, in order
	lastReq  request.Request
	resp     response.Response
	queryErr error
	stopped  bool

	// onQuery runs during Query, standing in for MCP tool calls the tutor makes
	// mid-turn.
	onQuery func()
}

func (f *fakeHarness) IsRunning() bool                     { return true }
func (f *fakeHarness) Start(context.Context, string) error { return nil }
func (f *fakeHarness) SupportsResume() bool                { return true }
func (f *fakeHarness) Stop() error                         { f.stopped = true; return nil }
func (f *fakeHarness) WriteSystemPrompt(_, content string) error {
	f.writes = append(f.writes, content)
	return nil
}
func (f *fakeHarness) Query(_ context.Context, req request.Request) (response.Response, error) {
	f.lastReq = req
	if f.onQuery != nil {
		f.onQuery()
	}
	return f.resp, f.queryErr
}

type fakeProgress struct{ data progress.Progress }

func (f *fakeProgress) Get() progress.Progress { return f.data }
func (f *fakeProgress) SetTrack(t string) error {
	f.data.CurrentTrack = t
	return nil
}
func (f *fakeProgress) StartSession(string) error      { return nil }
func (f *fakeProgress) SetFocus(v string) error        { f.data.Focus = v; return nil }
func (f *fakeProgress) SetSoulOverride(v string) error { f.data.SoulOverride = v; return nil }
func (f *fakeProgress) SetConceptState(id string, s progress.ConceptState) error {
	if f.data.Concepts == nil {
		f.data.Concepts = map[string]progress.ConceptRecord{}
	}
	f.data.Concepts[id] = progress.ConceptRecord{State: s}
	return nil
}

// ── Fixture ───────────────────────────────────────────────────────────────────

// newFixture builds a tutor over a temp data dir seeded with the given souls and
// a copy of the shipped programming-fundamentals plan.
func newFixture(t *testing.T, souls map[string]string, prog progress.Progress) (*Tutor, *fakeHarness) {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "MENTOR.md"), []byte("MENTOR CONTRACT"), 0644); err != nil {
		t.Fatal(err)
	}
	soulsDir := filepath.Join(dir, "souls")
	if err := os.MkdirAll(soulsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range souls {
		if err := os.WriteFile(filepath.Join(soulsDir, name+".md"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	plansDir := filepath.Join(dir, "lesson-plans")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../content/lesson-plans/programming-fundamentals.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "programming-fundamentals.md"), src, 0644); err != nil {
		t.Fatal(err)
	}

	h := &fakeHarness{resp: response.Response{Message: "What do you predict?"}}
	tut := &Tutor{
		cfg:      typeconfig.Config{DataDir: dir},
		harness:  h,
		progress: &fakeProgress{data: prog},
		plans:    lesson.NewLibrary(plansDir),
	}
	return tut, h
}

func allSouls() map[string]string {
	return map[string]string{
		"concepts-tutor":  "CONCEPTS SOUL",
		"debugging-coach": "DEBUGGING SOUL",
		"code-review":     "REVIEW SOUL",
	}
}

// demonstrated marks the first n concepts of programming-fundamentals done.
func demonstrated(n int) progress.Progress {
	p := progress.Progress{
		CurrentTrack: "programming-fundamentals",
		Concepts:     map[string]progress.ConceptRecord{},
	}
	for i := 1; i <= n; i++ {
		id := "PROG-00" + string(rune('0'+i))
		if i == 10 {
			id = "PROG-010"
		}
		p.Concepts[id] = progress.ConceptRecord{State: progress.StateDemonstrated}
	}
	return p
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestLoadSouls(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "concepts-tutor.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}

	souls, err := loadSouls(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := souls["concepts-tutor"]; !ok {
		t.Error("concepts-tutor was not loaded")
	}
	if len(souls) != 1 {
		t.Errorf("loaded %d souls, want 1 (non-markdown must be skipped)", len(souls))
	}
}

func TestSelectSoulDefaultsWhenNoTrackIsSet(t *testing.T) {
	tut, _ := newFixture(t, allSouls(), progress.Progress{})
	name, body := tut.selectSoul(allSouls())
	if name != "concepts-tutor" || body != "CONCEPTS SOUL" {
		t.Fatalf("selectSoul = %q/%q, want concepts-tutor", name, body)
	}
}

// The real payoff of parsing the plan: the soul follows the learner's position.
func TestSelectSoulFollowsThePlanAsConceptsAreDemonstrated(t *testing.T) {
	for _, tc := range []struct {
		done int
		want string
	}{
		{0, "concepts-tutor"},  // next is PROG-001
		{7, "concepts-tutor"},  // next is PROG-008
		{8, "debugging-coach"}, // next is PROG-009, per-concept Soul override
		{9, "code-review"},     // next is PROG-010
	} {
		tut, _ := newFixture(t, allSouls(), demonstrated(tc.done))
		name, _ := tut.selectSoul(allSouls())
		if name != tc.want {
			t.Errorf("%d demonstrated -> soul %q, want %q", tc.done, name, tc.want)
		}
	}
}

func TestSelectSoulFallsBackWhenMappedSoulFileIsMissing(t *testing.T) {
	souls := map[string]string{"concepts-tutor": "CONCEPTS SOUL"} // no debugging-coach
	tut, _ := newFixture(t, souls, demonstrated(8))               // next is PROG-009
	name, _ := tut.selectSoul(souls)
	if name != "concepts-tutor" {
		t.Fatalf("soul = %q, want fallback to concepts-tutor", name)
	}
}

// A learner who deletes or renames concepts-tutor.md should still get a persona.
func TestSelectSoulUsesAnySoulWhenTheDefaultIsGone(t *testing.T) {
	souls := map[string]string{"zeta": "Z", "alpha": "A"}
	tut, _ := newFixture(t, souls, progress.Progress{})
	name, body := tut.selectSoul(souls)
	if name != "alpha" || body != "A" {
		t.Fatalf("soul = %q/%q, want the alphabetically first soul", name, body)
	}
}

func TestSelectSoulReturnsEmptyWhenThereAreNoSouls(t *testing.T) {
	tut, _ := newFixture(t, map[string]string{}, progress.Progress{})
	if name, body := tut.selectSoul(map[string]string{}); name != "" || body != "" {
		t.Fatalf("soul = %q/%q, want empty", name, body)
	}
}

func TestSyncSystemPromptComposesMentorAndSoul(t *testing.T) {
	tut, h := newFixture(t, allSouls(), progress.Progress{})
	if _, err := tut.syncSystemPrompt(); err != nil {
		t.Fatal(err)
	}
	if len(h.writes) != 1 {
		t.Fatalf("%d writes, want 1", len(h.writes))
	}
	got := h.writes[0]
	if !strings.HasPrefix(got, "MENTOR CONTRACT") || !strings.Contains(got, "CONCEPTS SOUL") {
		t.Fatalf("composed prompt = %q", got)
	}
}

func TestSyncSystemPromptSkipsIdenticalRewrites(t *testing.T) {
	tut, h := newFixture(t, allSouls(), progress.Progress{})
	for i := 0; i < 3; i++ {
		if _, err := tut.syncSystemPrompt(); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.writes) != 1 {
		t.Fatalf("%d writes for unchanged content, want 1", len(h.writes))
	}
}

func TestSyncSystemPromptRewritesWhenTheSoulChanges(t *testing.T) {
	tut, h := newFixture(t, allSouls(), progress.Progress{})
	if _, err := tut.syncSystemPrompt(); err != nil {
		t.Fatal(err)
	}

	// Learner demonstrates through PROG-008; next concept is PROG-009.
	tut.progress = &fakeProgress{data: demonstrated(8)}
	name, err := tut.syncSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if name != "debugging-coach" {
		t.Fatalf("soul = %q, want debugging-coach", name)
	}
	if len(h.writes) != 2 {
		t.Fatalf("%d writes, want 2", len(h.writes))
	}
	if !strings.Contains(h.writes[1], "DEBUGGING SOUL") {
		t.Errorf("second write did not carry the new soul: %q", h.writes[1])
	}
}

// MENTOR.md is markdown the learner edits while the daemon runs.
func TestSyncSystemPromptPicksUpEditsToMentorMd(t *testing.T) {
	tut, h := newFixture(t, allSouls(), progress.Progress{})
	if _, err := tut.syncSystemPrompt(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tut.cfg.DataDir, "MENTOR.md"), []byte("REVISED CONTRACT"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := tut.syncSystemPrompt(); err != nil {
		t.Fatal(err)
	}
	if len(h.writes) != 2 || !strings.HasPrefix(h.writes[1], "REVISED CONTRACT") {
		t.Fatalf("edit to MENTOR.md was not picked up: %v", h.writes)
	}
}

func TestHandleFillsInTheCurrentConcept(t *testing.T) {
	tut, h := newFixture(t, allSouls(), demonstrated(3)) // next is PROG-004
	resp, err := tut.Handle(context.Background(), request.Request{
		Message: "why does this loop stop?", SessionID: "s1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if h.lastReq.ConceptID != "PROG-004" {
		t.Errorf("forwarded concept = %q, want PROG-004", h.lastReq.ConceptID)
	}
	if resp.ConceptID != "PROG-004" {
		t.Errorf("response concept = %q, want PROG-004", resp.ConceptID)
	}
}

func TestHandleKeepsAnExplicitConcept(t *testing.T) {
	tut, h := newFixture(t, allSouls(), demonstrated(3))
	if _, err := tut.Handle(context.Background(), request.Request{
		Message: "q", SessionID: "s1", ConceptID: "PROG-009",
	}); err != nil {
		t.Fatal(err)
	}
	if h.lastReq.ConceptID != "PROG-009" {
		t.Errorf("explicit concept was overwritten with %q", h.lastReq.ConceptID)
	}
}

func TestHandleRejectsAnEmptyMessage(t *testing.T) {
	tut, _ := newFixture(t, allSouls(), progress.Progress{})
	if _, err := tut.Handle(context.Background(), request.Request{SessionID: "s1"}); err == nil {
		t.Fatal("expected an error for an empty message")
	}
}

func TestStopStopsTheHarness(t *testing.T) {
	tut, h := newFixture(t, allSouls(), progress.Progress{})
	if err := tut.Stop(); err != nil {
		t.Fatal(err)
	}
	if !h.stopped {
		t.Error("harness was not stopped")
	}
}

// The syllabus cannot anticipate the session. A learner on a "values and types"
// concept who hits a nil dereference needs the debugging coach right then.
func TestSoulOverrideBeatsThePlan(t *testing.T) {
	prog := demonstrated(3) // next is PROG-004 -> concepts-tutor per the plan
	prog.SoulOverride = "debugging-coach"
	tut, _ := newFixture(t, allSouls(), prog)

	name, body := tut.selectSoul(allSouls())
	if name != "debugging-coach" || body != "DEBUGGING SOUL" {
		t.Fatalf("override ignored: got %q", name)
	}
}

func TestClearingTheOverrideReturnsToThePlan(t *testing.T) {
	prog := demonstrated(8) // next is PROG-009 -> debugging-coach per the plan
	prog.SoulOverride = "code-review"
	tut, _ := newFixture(t, allSouls(), prog)
	if name, _ := tut.selectSoul(allSouls()); name != "code-review" {
		t.Fatalf("override not applied: %q", name)
	}

	prog.SoulOverride = ""
	tut2, _ := newFixture(t, allSouls(), prog)
	if name, _ := tut2.selectSoul(allSouls()); name != "debugging-coach" {
		t.Fatalf("clearing the override did not restore the plan: %q", name)
	}
}

// An override naming a soul that does not exist must not leave the tutor
// personaless -- it falls through to normal selection.
func TestUnknownOverrideFallsThrough(t *testing.T) {
	prog := demonstrated(0)
	prog.SoulOverride = "nonexistent-soul"
	tut, _ := newFixture(t, allSouls(), prog)
	if name, _ := tut.selectSoul(allSouls()); name != "concepts-tutor" {
		t.Fatalf("got %q, want fallback to concepts-tutor", name)
	}
}

// Working with no lesson plan at all is a normal mode, not an error.
func TestTracklessSessionsStillWork(t *testing.T) {
	tut, h := newFixture(t, allSouls(), progress.Progress{})
	resp, err := tut.Handle(context.Background(), request.Request{
		Message: "why is my Terraform plan recreating the bucket?", SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("a trackless session failed: %v", err)
	}
	if h.lastReq.Message == "" {
		t.Error("the question was not forwarded")
	}
	if resp.Message == "" {
		t.Error("no reply")
	}
	if h.lastReq.ConceptID != "" {
		t.Errorf("invented a concept id with no track: %q", h.lastReq.ConceptID)
	}
}

func TestTracklessSessionStillComposesASoul(t *testing.T) {
	tut, h := newFixture(t, allSouls(), progress.Progress{})
	name, err := tut.syncSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if name == "" {
		t.Fatal("no soul chosen without a track")
	}
	if len(h.writes) != 1 || !strings.Contains(h.writes[0], "MENTOR CONTRACT") {
		t.Fatalf("no system prompt composed: %v", h.writes)
	}
}

// A persona the tutor chose for itself mid-turn must be live before the next
// turn starts, and must be what the daemon reports as active.
func TestPersonaChosenMidTurnTakesEffectImmediatelyAfterIt(t *testing.T) {
	prog := &fakeProgress{data: progress.Progress{}}
	tut, h := newFixture(t, allSouls(), progress.Progress{})
	tut.progress = prog

	// The harness stands in for the tutor calling set_soul through MCP.
	h.onQuery = func() { _ = prog.SetSoulOverride("debugging-coach") }

	if _, err := tut.Handle(context.Background(), request.Request{
		Message: "my worker pool deadlocks", SessionID: "s1",
	}); err != nil {
		t.Fatal(err)
	}

	if got := tut.ActiveSoul(); got != "debugging-coach" {
		t.Errorf("ActiveSoul() = %q, want debugging-coach", got)
	}
	last := h.writes[len(h.writes)-1]
	if !strings.Contains(last, "DEBUGGING SOUL") {
		t.Errorf("prompt was not recomposed for the new persona:\n%s", last)
	}
}
