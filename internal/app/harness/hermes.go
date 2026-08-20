package harness

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

type hermes struct {
	Base
}

func (h *hermes) Start(_ context.Context, _ string) error {
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("ollama binary not found in PATH (required for hermes): %w", err)
	}
	// Model selection and Ollama MCP integration not yet researched; start succeeds.
	// Query will fail until Query() is implemented.
	return nil
}

func (h *hermes) Query(_ context.Context, _ request.Request) (response.Response, error) {
	return response.Response{}, fmt.Errorf("hermes harness: Query not yet implemented -- contributions welcome")
}
