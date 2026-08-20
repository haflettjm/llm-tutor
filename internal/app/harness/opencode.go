package harness

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

type openCode struct {
	Base
}

func (o *openCode) Start(_ context.Context, _ string) error {
	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("opencode binary not found in PATH: %w", err)
	}
	// MCP registration format for opencode not yet researched; binary start succeeds.
	// Query will fail until Query() is implemented.
	return nil
}

func (o *openCode) Query(_ context.Context, _ request.Request) (response.Response, error) {
	return response.Response{}, fmt.Errorf("opencode harness: Query not yet implemented -- contributions welcome")
}
