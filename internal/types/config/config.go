package config

// Harness identifies which AI coding harness the tutor routes queries through.
type Harness string

const (
	ClaudeCode Harness = "claude-code"
	OpenCode   Harness = "opencode"
	Codex      Harness = "codex"
	Hermes     Harness = "hermes"
)

// SystemPromptFile returns the filename the harness reads as its system instructions.
func (h Harness) SystemPromptFile() string {
	switch h {
	case ClaudeCode:
		return "CLAUDE.md"
	case OpenCode:
		return "AGENTS.md"
	case Codex:
		return "CODEX.md"
	case Hermes:
		return "HERMES.md"
	default:
		return "AGENT.md"
	}
}

// Config is loaded from ~/Documents/llm-tutor/config.json.
type Config struct {
	Harness Harness `json:"harness"`            // claude-code | opencode | codex | hermes
	DataDir string  `json:"data_dir,omitempty"` // resolved at runtime
	Socket  string  `json:"socket,omitempty"`   // unix socket for editor plugin
	MCPAddr string  `json:"mcp_addr,omitempty"` // address our MCP server listens on
}
