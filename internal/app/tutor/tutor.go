package tutor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/haflettjm/llm-tutor/internal/app/harness"
	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/lesson"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

// defaultSoul teaches anything the active lesson plan does not assign a soul to.
const defaultSoul = "concepts-tutor"

// Service is the interface through which the HTTP layer routes tutor requests.
type Service interface {
	Handle(ctx context.Context, req request.Request) (response.Response, error)
	HandleStream(ctx context.Context, req request.Request, emit func(harness.StreamChunk) error) (response.Response, error)
}

// Tutor composes the system prompt, manages the harness session,
// and routes editor queries through it.
type Tutor struct {
	cfg      typeconfig.Config
	harness  harness.Harness
	progress progress.Repo
	plans    *lesson.Library

	// mu guards prompt composition so two concurrent editor turns cannot
	// interleave writes to the harness prompt file.
	mu         sync.Mutex
	lastPrompt string // last content written, to skip no-op rewrites
	activeSoul string
}

// New selects and starts the harness, then composes the initial system prompt.
func New(cfg typeconfig.Config, prog progress.Repo, plans *lesson.Library) (*Tutor, error) {
	h, err := harness.New(cfg)
	if err != nil {
		return nil, err
	}

	t := &Tutor{cfg: cfg, harness: h, progress: prog, plans: plans}

	if _, err := t.syncSystemPrompt(); err != nil {
		return nil, fmt.Errorf("compose system prompt: %w", err)
	}

	if !h.IsRunning() {
		if err := h.Start(context.Background(), cfg.MCPAddr); err != nil {
			return nil, fmt.Errorf("start %s harness: %w", cfg.Harness, err)
		}
	}
	return t, nil
}

// Handle routes one editor turn through the harness.
//
// The system prompt is re-composed before every turn rather than once at
// startup: the learner's concept can advance mid-session (which changes the
// soul), and MENTOR.md and the soul files are plain markdown the learner is
// expected to edit while the daemon runs.
func (t *Tutor) Handle(ctx context.Context, req request.Request) (response.Response, error) {
	if req.Message == "" {
		return response.Response{}, fmt.Errorf("request has no message")
	}

	soul, err := t.syncSystemPrompt()
	if err != nil {
		return response.Response{}, err
	}

	// Session and concept bookkeeping is best-effort. A failure to record
	// which session this is must not cost the learner their answer.
	_ = t.progress.StartSession(req.SessionID)
	if req.ConceptID == "" {
		req.ConceptID = t.currentConceptID()
	}

	resp, err := t.harness.Query(ctx, req)
	if err != nil {
		return response.Response{}, err
	}
	if resp.ConceptID == "" {
		resp.ConceptID = req.ConceptID
	}
	t.setActiveSoul(soul)

	// Re-compose now that the turn is over. The tutor may have changed its own
	// persona or advanced a concept mid-turn through MCP, and those calls cannot
	// affect the prompt they were made under. Doing it here rather than lazily
	// on the next turn keeps what the daemon reports as active honest, and means
	// the next turn starts with the prompt already correct.
	// Best effort: a failure here must not cost the learner an answer they have.
	if next, syncErr := t.syncSystemPrompt(); syncErr == nil {
		t.setActiveSoul(next)
	}
	return resp, nil
}

func (t *Tutor) HandleStream(ctx context.Context, req request.Request, emit func(harness.StreamChunk) error) (response.Response, error) {
	if req.Message == "" {
		return response.Response{}, fmt.Errorf("request has no message")
	}
	soul, err := t.syncSystemPrompt()
	if err != nil {
		return response.Response{}, err
	}
	_ = t.progress.StartSession(req.SessionID)
	if req.ConceptID == "" {
		req.ConceptID = t.currentConceptID()
	}

	var resp response.Response
	if streamer, ok := harness.CanStream(t.harness); ok {
		resp, err = streamer.StreamQuery(ctx, req, emit)
	} else {
		resp, err = t.harness.Query(ctx, req)
		if err == nil && emit != nil {
			err = emit(harness.StreamChunk{Text: resp.Message})
		}
	}
	if err != nil {
		return response.Response{}, err
	}
	if resp.ConceptID == "" {
		resp.ConceptID = req.ConceptID
	}
	t.setActiveSoul(soul)
	if next, syncErr := t.syncSystemPrompt(); syncErr == nil {
		t.setActiveSoul(next)
	}
	return resp, nil
}

// Stop shuts the harness down.
func (t *Tutor) Stop() error { return t.harness.Stop() }

// ActiveSoul reports which soul composed the most recent system prompt.
func (t *Tutor) ActiveSoul() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.activeSoul
}

func (t *Tutor) setActiveSoul(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.activeSoul = name
}

// currentConceptID is the concept the learner is working on: the first one in
// the active track they have not demonstrated. Empty when no track is set.
func (t *Tutor) currentConceptID() string {
	prog := t.progress.Get()
	if prog.CurrentTrack == "" {
		return ""
	}
	plan, err := t.plans.Plan(prog.CurrentTrack)
	if err != nil {
		return ""
	}
	if next, ok := plan.NextIncomplete(prog.Demonstrated); ok {
		return next.ID
	}
	return ""
}

// selectSoul resolves which soul should be teaching right now.
//
// Resolution order:
//  1. an explicit override the tutor set for itself (see the set_soul tool)
//  2. the active plan's mapping for the current concept
//  3. defaultSoul
//  4. any soul at all
//
// The override comes first because the syllabus cannot anticipate the session.
// A learner on a "values and types" concept who hits a nil dereference needs
// the debugging coach right then, not four concepts from now.
//
// Step 4 matters because a learner who renames or deletes concepts-tutor.md
// should still get a tutor rather than a bare MENTOR.md with no persona.
func (t *Tutor) selectSoul(souls map[string]string) (name, body string) {
	if want := t.progress.Get().SoulOverride; want != "" {
		if body, ok := souls[want]; ok {
			return want, body
		}
	}
	if conceptID := t.currentConceptID(); conceptID != "" {
		prog := t.progress.Get()
		if plan, err := t.plans.Plan(prog.CurrentTrack); err == nil {
			if want := plan.SoulFor(conceptID); want != "" {
				if body, ok := souls[want]; ok {
					return want, body
				}
			}
		}
	}
	if body, ok := souls[defaultSoul]; ok {
		return defaultSoul, body
	}
	for _, name := range sortedKeys(souls) {
		return name, souls[name]
	}
	return "", ""
}

// syncSystemPrompt composes MENTOR.md + the active soul and writes it to the
// file the configured harness reads. Writing is skipped when the content is
// byte-identical to the last write, so an unchanged turn does not touch the
// file the harness may be reading.
func (t *Tutor) syncSystemPrompt() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	mentor, err := os.ReadFile(filepath.Join(t.cfg.DataDir, "MENTOR.md"))
	if err != nil {
		return "", fmt.Errorf("read MENTOR.md: %w", err)
	}
	souls, err := loadSouls(filepath.Join(t.cfg.DataDir, "souls"))
	if err != nil {
		return "", err
	}

	name, body := t.selectSoul(souls)

	var sb strings.Builder
	sb.Write(mentor)
	if body != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(body)
	}
	content := sb.String()

	if content == t.lastPrompt {
		return name, nil
	}
	if err := t.harness.WriteSystemPrompt(t.cfg.DataDir, content); err != nil {
		return "", err
	}
	t.lastPrompt = content
	t.activeSoul = name
	return name, nil
}

func loadSouls(dir string) (map[string]string, error) {
	souls := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read souls dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read soul %s: %w", e.Name(), err)
		}
		souls[strings.TrimSuffix(e.Name(), ".md")] = string(data)
	}
	return souls, nil
}

// sortedKeys keeps the "any soul at all" fallback deterministic across restarts.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
