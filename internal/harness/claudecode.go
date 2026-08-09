package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/haflettjm/llm-tutor/internal/types"
)

// claudeCode routes queries through the Claude Code CLI (claude).
// System prompt: written to CLAUDE.md in the working directory.
// MCP: registered via ~/.claude.json mcpServers entry before Start.
type claudeCode struct {
	cmd *exec.Cmd
}

func (c *claudeCode) IsRunning() bool {
	// TODO: check for a PID file or probe the socket claude exposes in headless mode.
	return false
}

func (c *claudeCode) WriteSystemPrompt(dir, content string) error {
	return os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0644)
}

// Start registers our MCP server in ~/.claude.json then launches claude headless.
func (c *claudeCode) Start(ctx context.Context, mcpAddr string) error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude binary not found in PATH: %w", err)
	}
	if err := registerClaudeMCP(mcpAddr); err != nil {
		return fmt.Errorf("register MCP server with claude: %w", err)
	}
	// TODO: research the correct flags for claude headless/daemon mode.
	// Candidate: `claude --dangerously-skip-permissions` or an API server flag.
	return fmt.Errorf("claude-code headless start: flags not yet confirmed -- research required")
}

func (c *claudeCode) Query(ctx context.Context, req types.Request) (types.Response, error) {
	// TODO: route through the running claude session (stdin/stdout pipe or HTTP).
	return types.Response{}, fmt.Errorf("claude-code query: not yet implemented")
}

func (c *claudeCode) Stop() error {
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

// registerClaudeMCP writes our MCP server entry into ~/.claude.json.
func registerClaudeMCP(mcpAddr string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude.json")

	var root map[string]json.RawMessage
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &root)
	}
	if root == nil {
		root = make(map[string]json.RawMessage)
	}

	servers := map[string]any{
		"llm-tutor": map[string]string{
			"type": "sse",
			"url":  "http://localhost" + mcpAddr + "/sse",
		},
	}
	// Merge into existing mcpServers if present.
	if existing, ok := root["mcpServers"]; ok {
		var prev map[string]json.RawMessage
		if err := json.Unmarshal(existing, &prev); err == nil {
			for k, v := range prev {
				if k != "llm-tutor" {
					servers[k] = v
				}
			}
		}
	}

	raw, _ := json.Marshal(servers)
	root["mcpServers"] = raw
	out, _ := json.MarshalIndent(root, "", "  ")
	return os.WriteFile(path, append(out, '\n'), 0644)
}
