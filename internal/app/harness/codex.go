package harness

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

type codex struct {
	Base
}

func (c *codex) Start(_ context.Context, _ string) error {
	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("codex binary not found in PATH: %w", err)
	}
	// MCP registration format for codex not yet researched; binary start succeeds.
	// Query will fail until Query() is implemented.
	return nil
}

func (c *codex) Query(_ context.Context, _ request.Request) (response.Response, error) {
	return response.Response{}, fmt.Errorf("codex harness: Query not yet implemented -- contributions welcome")
}
