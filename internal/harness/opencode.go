package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/haflettjm/llm-tutor/internal/types"
)

// openCode routes queries through the opencode CLI.
// System prompt: written to AGENTS.md in the working directory.
// MCP: registered via opencode's config before Start.
type openCode struct {
	cmd *exec.Cmd
}

func (o *openCode) IsRunning() bool {
	return false // TODO: PID file or socket probe
}

func (o *openCode) WriteSystemPrompt(dir, content string) error {
	return os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644)
}

func (o *openCode) Start(ctx context.Context, mcpAddr string) error {
	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("opencode binary not found in PATH: %w", err)
	}
	// TODO: research opencode headless flags and MCP server registration format.
	return fmt.Errorf("opencode headless start: flags not yet confirmed -- research required")
}

func (o *openCode) Query(ctx context.Context, req types.Request) (types.Response, error) {
	return types.Response{}, fmt.Errorf("opencode query: not yet implemented")
}

func (o *openCode) Stop() error {
	if o.cmd != nil && o.cmd.Process != nil {
		return o.cmd.Process.Kill()
	}
	return nil
}
