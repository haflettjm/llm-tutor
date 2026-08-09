package tutor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/haflettjm/llm-tutor/internal/harness"
	"github.com/haflettjm/llm-tutor/internal/progress"
	"github.com/haflettjm/llm-tutor/internal/types"
)

// Tutor composes the system prompt, manages the harness session,
// and routes editor queries through it.
type Tutor struct {
	cfg      types.Config
	harness  harness.Harness
	mentor   string
	souls    map[string]string
	progress *progress.Store
}

// New loads MENTOR.md and souls from the data directory, selects and starts the harness.
func New(cfg types.Config, prog *progress.Store) (*Tutor, error) {
	mentor, err := os.ReadFile(filepath.Join(cfg.DataDir, "MENTOR.md"))
	if err != nil {
		return nil, fmt.Errorf("read MENTOR.md: %w", err)
	}

	souls, err := loadSouls(filepath.Join(cfg.DataDir, "souls"))
	if err != nil {
		return nil, err
	}

	h, err := harness.New(cfg.Harness)
	if err != nil {
		return nil, err
	}

	t := &Tutor{
		cfg:      cfg,
		harness:  h,
		mentor:   string(mentor),
		souls:    souls,
		progress: prog,
	}

	if err := t.injectSystemPrompt(); err != nil {
		return nil, fmt.Errorf("inject system prompt: %w", err)
	}

	if !h.IsRunning() {
		if err := h.Start(context.Background(), cfg.MCPAddr); err != nil {
			return nil, fmt.Errorf("start %s harness: %w", cfg.Harness, err)
		}
	}

	return t, nil
}

// injectSystemPrompt composes MENTOR.md + the active soul and writes it to the
// file the configured harness reads as its system instructions.
func (t *Tutor) injectSystemPrompt() error {
	soul := t.selectSoul()
	var sb strings.Builder
	sb.WriteString(t.mentor)
	if soul != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(soul)
	}
	return t.harness.WriteSystemPrompt(t.cfg.DataDir, sb.String())
}

// selectSoul picks the soul based on the learner's current concept track position.
// Falls back to concepts-tutor when no better match is found.
func (t *Tutor) selectSoul() string {
	prog := t.progress.Get()
	// Concept-to-soul mapping lives in the lesson plan; for now fall back to defaults.
	switch prog.CurrentTrack {
	default:
		if soul, ok := t.souls["concepts-tutor"]; ok {
			return soul
		}
	}
	return ""
}

// Handle routes one editor turn through the harness and returns its response.
func (t *Tutor) Handle(ctx context.Context, req types.Request) (types.Response, error) {
	return t.harness.Query(ctx, req)
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
