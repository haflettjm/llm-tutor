package harness

import (
	"context"
	"fmt"

	"github.com/haflettjm/llm-tutor/internal/types"
)

// Harness manages a headless AI coding harness that the tutor routes queries through.
type Harness interface {
	// IsRunning returns true if the harness is already running as a headless server.
	IsRunning() bool
	// Start launches the harness headless, registering our MCP server so the
	// harness can call back into the tutor's lesson and progress tools.
	Start(ctx context.Context, mcpAddr string) error
	// WriteSystemPrompt composes MENTOR.md + soul content and writes it to the
	// file the harness reads as its system prompt (e.g. CLAUDE.md, AGENTS.md).
	WriteSystemPrompt(dir, content string) error
	// Query sends one turn through the harness and returns its response.
	Query(ctx context.Context, req types.Request) (types.Response, error)
	// Stop shuts down the harness process.
	Stop() error
}

// New returns the Harness implementation for the given harness type.
func New(h types.Harness) (Harness, error) {
	switch h {
	case types.HarnessClaudeCode:
		return &claudeCode{}, nil
	case types.HarnessOpenCode:
		return &openCode{}, nil
	case types.HarnessCodex:
		return &codex{}, nil
	case types.HarnessHermes:
		return &hermes{}, nil
	default:
		return nil, fmt.Errorf("unknown harness %q -- valid values: claude-code, opencode, codex, hermes", h)
	}
}
