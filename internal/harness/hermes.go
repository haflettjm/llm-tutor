package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/haflettjm/llm-tutor/internal/types"
)

// hermes routes queries through a Hermes-compatible harness (local model via Ollama or similar).
// System prompt: written to HERMES.md in the working directory.
type hermes struct {
	cmd *exec.Cmd
}

func (h *hermes) IsRunning() bool {
	return false // TODO: probe Ollama or Hermes endpoint
}

func (h *hermes) WriteSystemPrompt(dir, content string) error {
	return os.WriteFile(filepath.Join(dir, "HERMES.md"), []byte(content), 0644)
}

func (h *hermes) Start(ctx context.Context, mcpAddr string) error {
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("ollama binary not found in PATH (required for hermes): %w", err)
	}
	// TODO: determine correct model name and MCP registration for Hermes via Ollama.
	return fmt.Errorf("hermes headless start: model selection and MCP config not yet confirmed -- research required")
}

func (h *hermes) Query(ctx context.Context, req types.Request) (types.Response, error) {
	return types.Response{}, fmt.Errorf("hermes query: not yet implemented")
}

func (h *hermes) Stop() error {
	if h.cmd != nil && h.cmd.Process != nil {
		return h.cmd.Process.Kill()
	}
	return nil
}
