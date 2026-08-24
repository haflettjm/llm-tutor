package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/lesson"
	"github.com/haflettjm/llm-tutor/internal/types/profile"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
	"github.com/haflettjm/llm-tutor/internal/types/status"
)

// ── Fakes ─────────────────────────────────────────────────────────────────────

type fakeTutor struct {
	req  request.Request
	resp response.Response
	err  error
}

func (f *fakeTutor) Handle(_ context.Context, req request.Request) (response.Response, error) {
	f.req = req
	return f.resp, f.err
}

type fakeProgress struct{ data progress.Progress }

func (f *fakeProgress) Get() progress.Progress         { return f.data }
func (f *fakeProgress) StartSession(string) error      { return nil }
func (f *fakeProgress) SetFocus(v string) error        { f.data.Focus = v; return nil }
func (f *fakeProgress) SetSoulOverride(v string) error { f.data.SoulOverride = v; return nil }
func (f *fakeProgress) SetTrack(t string) error        { f.data.CurrentTrack = t; return nil }
func (f *fakeProgress) SetConceptState(id string, s progress.ConceptState) error {
	if f.data.Concepts == nil {
		f.data.Concepts = map[string]progress.ConceptRecord{}
	}
	f.data.Concepts[id] = progress.ConceptRecord{State: s}
	return nil
}

type fakeProfile struct{ data profile.LearnerProfile }

func (f *fakeProfile) Get() profile.LearnerProfile                    { return f.data }
func (f *fakeProfile) UpdateIdentity(_, _, _ string) error            { return nil }
func (f *fakeProfile) UpdateContext(_, _, _ string, _ []string) error { return nil }
func (f *fakeProfile) AddSessionNote(profile.SessionNote) error       { return nil }
func (f *fakeProfile) UpdateStyle(_, _, _, _ string, _, _ []string, _, _, _ float64) error {
	return nil
}

// ── Fixture ───────────────────────────────────────────────────────────────────

func newRouter(t *testing.T, prog progress.Progress) (http.Handler, *fakeTutor, *fakeProgress) {
	t.Helper()
	dir := t.TempDir()
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

	tut := &fakeTutor{resp: response.Response{Message: "What do you predict?", ResponseType: "question"}}
	pr := &fakeProgress{data: prog}
	r := Router(Deps{
		Cfg:        typeconfig.Config{Harness: typeconfig.ClaudeCode, DataDir: dir},
		Tutor:      tut,
		Progress:   pr,
		Profile:    &fakeProfile{},
		Plans:      lesson.NewLibrary(plansDir),
		ActiveSoul: func() string { return "concepts-tutor" },
	})
	return r, tut, pr
}

func get(t *testing.T, h http.Handler, path string, out any) int {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if out != nil && w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s: %v (body %s)", path, err, w.Body.String())
		}
	}
	return w.Code
}

func post(t *testing.T, h http.Handler, path string, body, out any) int {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if out != nil {
		_ = json.Unmarshal(w.Body.Bytes(), out)
	}
	return w.Code
}

func trackAt(n int) progress.Progress {
	p := progress.Progress{CurrentTrack: "programming-fundamentals", Concepts: map[string]progress.ConceptRecord{}, Sessions: 4}
	ids := []string{"PROG-001", "PROG-002", "PROG-003", "PROG-004", "PROG-005",
		"PROG-006", "PROG-007", "PROG-008", "PROG-009", "PROG-010"}
	for i := 0; i < n && i < len(ids); i++ {
		p.Concepts[ids[i]] = progress.ConceptRecord{State: progress.StateDemonstrated}
	}
	return p
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHealthReportsTheLiveConfiguration(t *testing.T) {
	r, _, _ := newRouter(t, progress.Progress{})
	var h status.Health
	if code := get(t, r, "/health", &h); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if h.Status != "ok" || h.Harness != "claude-code" || h.ActiveSoul != "concepts-tutor" {
		t.Fatalf("health = %+v", h)
	}
	if h.LessonPlans != 1 {
		t.Errorf("lesson_plans = %d, want 1", h.LessonPlans)
	}
}

func TestTutorRoutesTheTurn(t *testing.T) {
	r, tut, _ := newRouter(t, progress.Progress{})
	var resp response.Response
	code := post(t, r, "/tutor", request.Request{Message: "why?", SessionID: "s1", Language: "go"}, &resp)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if tut.req.Message != "why?" || tut.req.Language != "go" {
		t.Errorf("forwarded = %+v", tut.req)
	}
	if resp.Message != "What do you predict?" {
		t.Errorf("response = %+v", resp)
	}
}

func TestTutorRejectsMalformedJSON(t *testing.T) {
	r, _, _ := newRouter(t, progress.Progress{})
	req := httptest.NewRequest(http.MethodPost, "/tutor", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestProgressCountsAndLocatesTheNextConcept(t *testing.T) {
	r, _, _ := newRouter(t, trackAt(3))
	var p status.Progress
	if code := get(t, r, "/progress", &p); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if p.Track != "programming-fundamentals" || p.TrackTitle != "Programming Fundamentals" {
		t.Errorf("track = %q / %q", p.Track, p.TrackTitle)
	}
	if p.Demonstrated != 3 || p.Total != 10 {
		t.Errorf("demonstrated %d of %d, want 3 of 10", p.Demonstrated, p.Total)
	}
	if p.NextConcept == nil || p.NextConcept.ID != "PROG-004" {
		t.Fatalf("next concept = %+v, want PROG-004", p.NextConcept)
	}
	if p.Position != 4 {
		t.Errorf("position = %d, want 4", p.Position)
	}
	if p.Sessions != 4 {
		t.Errorf("sessions = %d", p.Sessions)
	}
}

func TestProgressWithNoTrackExplainsWhatToDo(t *testing.T) {
	r, _, _ := newRouter(t, progress.Progress{})
	var p status.Progress
	get(t, r, "/progress", &p)
	if p.Note == "" {
		t.Fatal("no guidance when there is no track")
	}
	if p.NextConcept != nil {
		t.Errorf("next concept reported without a track: %+v", p.NextConcept)
	}
}

func TestProgressWhenTrackNamesAMissingPlan(t *testing.T) {
	r, _, _ := newRouter(t, progress.Progress{CurrentTrack: "deleted-track"})
	var p status.Progress
	if code := get(t, r, "/progress", &p); code != http.StatusOK {
		t.Fatalf("status = %d -- a stale track must not 500", code)
	}
	if p.Note == "" {
		t.Error("no explanation for the missing plan")
	}
}

func TestProgressWhenEverythingIsDemonstrated(t *testing.T) {
	r, _, _ := newRouter(t, trackAt(10))
	var p status.Progress
	get(t, r, "/progress", &p)
	if p.NextConcept != nil {
		t.Errorf("next concept offered after completion: %+v", p.NextConcept)
	}
	if p.Demonstrated != 10 || p.Note == "" {
		t.Errorf("completion not reported: %+v", p)
	}
}

func TestPlansListsTracksAndMarksTheActiveOne(t *testing.T) {
	r, _, _ := newRouter(t, trackAt(0))
	var p status.Plans
	if code := get(t, r, "/plans", &p); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(p.Plans) != 1 || p.Plans[0].ID != "programming-fundamentals" {
		t.Fatalf("plans = %+v", p.Plans)
	}
	if p.Active != "programming-fundamentals" {
		t.Errorf("active = %q", p.Active)
	}
	if p.Plans[0].Concepts != 10 {
		t.Errorf("concept count = %d", p.Plans[0].Concepts)
	}
}

func TestSetTrackSwitchesAndReturnsTheNewProgress(t *testing.T) {
	r, _, prog := newRouter(t, progress.Progress{})
	var p status.Progress
	code := post(t, r, "/track", status.TrackRequest{Track: "programming-fundamentals"}, &p)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if prog.data.CurrentTrack != "programming-fundamentals" {
		t.Errorf("track was not set, got %q", prog.data.CurrentTrack)
	}
	if p.NextConcept == nil || p.NextConcept.ID != "PROG-001" {
		t.Errorf("new track should start at PROG-001, got %+v", p.NextConcept)
	}
}

func TestSetUnknownTrackIs404AndDoesNotChangeState(t *testing.T) {
	r, _, prog := newRouter(t, progress.Progress{CurrentTrack: "programming-fundamentals"})
	var e status.Error
	code := post(t, r, "/track", status.TrackRequest{Track: "does-not-exist"}, &e)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if e.Error == "" {
		t.Error("no error message returned")
	}
	if prog.data.CurrentTrack != "programming-fundamentals" {
		t.Errorf("track changed to %q on a failed switch", prog.data.CurrentTrack)
	}
}

// A track whose concepts are still being written is not a finished track.
func TestProgressDistinguishesAnEmptyTrackFromACompletedOne(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "lesson-plans")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatal(err)
	}
	empty := "---\nid: draft\ntitle: Draft Track\n---\n\n## Concepts\n\nNothing written yet.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "draft.md"), []byte(empty), 0644); err != nil {
		t.Fatal(err)
	}

	r := Router(Deps{
		Cfg:      typeconfig.Config{Harness: typeconfig.ClaudeCode, DataDir: dir},
		Tutor:    &fakeTutor{},
		Progress: &fakeProgress{data: progress.Progress{CurrentTrack: "draft"}},
		Profile:  &fakeProfile{},
		Plans:    lesson.NewLibrary(plansDir),
	})

	var p status.Progress
	get(t, r, "/progress", &p)
	if p.NextConcept != nil {
		t.Errorf("empty track offered a concept: %+v", p.NextConcept)
	}
	if p.Total != 0 {
		t.Errorf("total = %d, want 0", p.Total)
	}
	if !strings.Contains(p.Note, "no concept-level entries") {
		t.Errorf("empty track reported as complete: %q", p.Note)
	}
}

// No track is a normal state. The wording must not read as an error.
func TestNoTrackIsNotFramedAsAProblem(t *testing.T) {
	r, _, _ := newRouter(t, progress.Progress{})
	var p status.Progress
	get(t, r, "/progress", &p)
	if !strings.Contains(p.Note, "that is fine") {
		t.Errorf("trackless state reads as an error: %q", p.Note)
	}
}

// Work the learner brought themselves must be counted, not discarded.
func TestOffPlanConceptsAreCounted(t *testing.T) {
	prog := progress.Progress{
		CurrentTrack: "programming-fundamentals",
		Concepts: map[string]progress.ConceptRecord{
			"PROG-001":       {State: progress.StateDemonstrated},
			"SQL-INDEXES":    {State: progress.StateDemonstrated},
			"TF-STATE-DRIFT": {State: progress.StateLearning},
		},
	}
	r, _, _ := newRouter(t, prog)
	var p status.Progress
	get(t, r, "/progress", &p)

	if p.OffPlan != 2 {
		t.Errorf("off_plan_concepts = %d, want 2", p.OffPlan)
	}
	if p.Demonstrated != 1 {
		t.Errorf("demonstrated = %d, want 1 (only track concepts count toward the track)", p.Demonstrated)
	}
}

func TestFocusIsReported(t *testing.T) {
	r, _, _ := newRouter(t, progress.Progress{Focus: "goroutine leak in the worker pool"})
	var p status.Progress
	get(t, r, "/progress", &p)
	if p.Focus != "goroutine leak in the worker pool" {
		t.Errorf("focus = %q", p.Focus)
	}
}
