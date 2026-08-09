package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/haflettjm/llm-tutor/internal/types"
)

// codex routes queries through the OpenAI Codex CLI.
// System prompt: written to CODEX.md in the working directory.
type codex struct {
	cmd *exec.Cmd
}

func (c *codex) IsRunning() bool {
	return false // TODO: PID file or socket probe
}

func (c *codex) WriteSystemPrompt(dir, content string) error {
	return os.WriteFile(filepath.Join(dir, "CODEX.md"), []byte(content), 0644)
}

func (c *codex) Start(ctx context.Context, mcpAddr string) error {
	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("codex binary not found in PATH: %w", err)
	}
	// TODO: research codex headless flags and MCP server registration format.
	return fmt.Errorf("codex headless start: flags not yet confirmed -- research required")
}

func (c *codex) Query(ctx context.Context, req types.Request) (types.Response, error) {
	return types.Response{}, fmt.Errorf("codex query: not yet implemented")
}

func (c *codex) Stop() error {
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
