package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

// Harness manages a headless AI coding harness that the tutor routes queries through.
type Harness interface {
	IsRunning() bool
	Start(ctx context.Context, mcpAddr string) error
	WriteSystemPrompt(dir, content string) error
	Query(ctx context.Context, req request.Request) (response.Response, error)
	Stop() error

	// SupportsResume reports whether the underlying CLI can continue a prior
	// conversation on its own. Harnesses that return false must have their
	// conversation history replayed into each prompt by the caller; see
	// docs on the session model in the plan.
	SupportsResume() bool
}

// StreamChunk is one incremental piece of a tutor reply. Reset clears any text
// already rendered for the current reply before appending Text.
type StreamChunk struct {
	Text  string
	Reset bool
}

// Streamer is implemented by harnesses that can emit a tutor reply incrementally.
type Streamer interface {
	StreamQuery(ctx context.Context, req request.Request, emit func(StreamChunk) error) (response.Response, error)
}

// CanStream reports whether h supports incremental replies.
func CanStream(h Harness) (Streamer, bool) {
	s, ok := h.(Streamer)
	return s, ok
}

// Base is embedded in every concrete harness. It provides shared state and default
// implementations for IsRunning, WriteSystemPrompt, SupportsResume, and Stop.
// Each harness only needs to implement Start and Query.
type Base struct {
	mu         sync.RWMutex
	cmd        *exec.Cmd
	started    bool
	promptFile string // filename the harness reads as its system prompt (e.g. CLAUDE.md)
}

// IsRunning reports whether Start has completed successfully. For harnesses that
// invoke a CLI per turn rather than holding a long-lived process, this means
// "initialized and ready" rather than "a process is alive".
func (b *Base) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.cmd != nil && b.cmd.Process != nil {
		return true
	}
	return b.started
}

// markStarted records that Start completed, so callers can avoid repeating
// one-time setup such as MCP registration.
func (b *Base) markStarted() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.started = true
}

// SupportsResume defaults to false. Harnesses whose CLI can continue a prior
// conversation override this.
func (b *Base) SupportsResume() bool { return false }

// WriteSystemPrompt writes the composed MENTOR.md + soul content to the
// harness-specific file in dir.
func (b *Base) WriteSystemPrompt(dir, content string) error {
	return os.WriteFile(filepath.Join(dir, b.promptFile), []byte(content), 0644)
}

// Stop kills the harness process if one was started by this instance.
func (b *Base) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.started = false
	if b.cmd != nil && b.cmd.Process != nil {
		return b.cmd.Process.Kill()
	}
	return nil
}

// New returns the Harness implementation for the configured harness type.
func New(cfg typeconfig.Config) (Harness, error) {
	switch cfg.Harness {
	case typeconfig.ClaudeCode:
		return newClaudeCode(cfg.DataDir), nil
	case typeconfig.OpenCode:
		return &openCode{Base: Base{promptFile: "AGENTS.md"}}, nil
	case typeconfig.Codex:
		return &codex{Base: Base{promptFile: "CODEX.md"}}, nil
	case typeconfig.Hermes:
		return &hermes{Base: Base{promptFile: "HERMES.md"}}, nil
	default:
		return nil, fmt.Errorf("unknown harness %q -- valid: claude-code, opencode, codex, hermes", cfg.Harness)
	}
}
