package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
)

const (
	defaultSocket  = "/tmp/llm-tutor.sock"
	defaultMCPAddr = ":7890"
)

// Load reads ~/Documents/llm-tutor/config.json.
// On first run (file absent) it seeds a template and returns an error
// telling the user to set their preferred harness before starting.
func Load() (typeconfig.Config, error) {
	dataDir := resolveDataDir()
	path := filepath.Join(dataDir, "config.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if seedErr := seedTemplate(dataDir, path); seedErr != nil {
			return typeconfig.Config{}, fmt.Errorf("seed config template: %w", seedErr)
		}
		return typeconfig.Config{}, fmt.Errorf(
			"no config found -- %s has been created, set \"harness\" to one of: claude-code, opencode, codex, hermes",
			path,
		)
	}
	if err != nil {
		return typeconfig.Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg typeconfig.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return typeconfig.Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Harness == "" {
		return typeconfig.Config{}, fmt.Errorf(
			"config: harness is required -- edit %s and set one of: claude-code, opencode, codex, hermes",
			path,
		)
	}

	cfg.DataDir = dataDir
	if cfg.Socket == "" {
		cfg.Socket = defaultSocket
	}
	if cfg.MCPAddr == "" {
		cfg.MCPAddr = defaultMCPAddr
	}

	return cfg, nil
}

func resolveDataDir() string {
	if v := os.Getenv("LLM_TUTOR_DATA"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents", "llm-tutor")
}

func seedTemplate(dataDir, path string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	tmpl := struct {
		Harness string `json:"harness"`
		Socket  string `json:"socket"`
		MCPAddr string `json:"mcp_addr"`
	}{
		Harness: string(typeconfig.ClaudeCode),
		Socket:  defaultSocket,
		MCPAddr: defaultMCPAddr,
	}
	data, _ := json.MarshalIndent(tmpl, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}
