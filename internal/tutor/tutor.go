package tutor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Request is what the editor plugin sends on every chat turn.
type Request struct {
	Message   string `json:"message"`
	Diff      string `json:"diff"`
	Language  string `json:"language"`
	ConceptID string `json:"concept_id,omitempty"`
	SessionID string `json:"session_id"`
}

// Response is what the backend returns to the editor plugin.
type Response struct {
	Message      string `json:"message"`
	ResponseType string `json:"response_type"` // question, observation, hint, explanation
	ConceptID    string `json:"concept_id,omitempty"`
	HintLevel    int    `json:"hint_level"`
}

// Tutor holds the loaded teaching contract and souls.
type Tutor struct {
	apiKey     string
	mentorDoc  string
	souls      map[string]string // soul name -> file content
}

// New loads MENTOR.md and all soul files from disk.
func New(apiKey, mentorPath, soulsDir string) (*Tutor, error) {
	mentor, err := os.ReadFile(mentorPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", mentorPath, err)
	}

	souls := make(map[string]string)
	entries, err := os.ReadDir(soulsDir)
	if err != nil {
		return nil, fmt.Errorf("read souls dir %s: %w", soulsDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(soulsDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read soul %s: %w", e.Name(), err)
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		souls[name] = string(data)
	}

	return &Tutor{
		apiKey:    apiKey,
		mentorDoc: string(mentor),
		souls:     souls,
	}, nil
}

// selectSoul picks the right soul for the request.
// Phase 4 implementation: map concept ID to soul, fall back to language/diff heuristics.
func (t *Tutor) selectSoul(_ Request) string {
	if soul, ok := t.souls["concepts-tutor"]; ok {
		return soul
	}
	return ""
}

// Handle processes one tutor turn.
// Phase 4 implementation: compose system prompt, call Anthropic API, parse response.
func (t *Tutor) Handle(ctx context.Context, req Request) (Response, error) {
	_ = t.mentorDoc
	_ = t.selectSoul(req)
	// TODO Phase 4: call Anthropic API with composed system prompt.
	return Response{
		Message:      "not implemented",
		ResponseType: "question",
		HintLevel:    0,
	}, nil
}
