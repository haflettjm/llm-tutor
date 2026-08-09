package setup

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/haflettjm/llm-tutor/internal/content"
	"github.com/haflettjm/llm-tutor/internal/types"
)

// Run creates the data directory, seeds default content files, and initializes
// state files. Existing user-edited files are never overwritten.
func Run(cfg types.Config) error {
	for _, d := range []string{
		cfg.DataDir,
		filepath.Join(cfg.DataDir, "souls"),
		filepath.Join(cfg.DataDir, "lesson-plans"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	if err := seedFile(filepath.Join(cfg.DataDir, "MENTOR.md"), content.MentorMD); err != nil {
		return err
	}
	if err := seedFS(cfg.DataDir, content.SoulsFS, "souls"); err != nil {
		return err
	}
	if err := seedFS(cfg.DataDir, content.LessonPlansFS, "lesson-plans"); err != nil {
		return err
	}
	if err := touchFile(filepath.Join(cfg.DataDir, "learning-events.jsonl")); err != nil {
		return err
	}
	return initProgress(filepath.Join(cfg.DataDir, "progress.json"))
}

// seedFile writes data to dst only if dst does not already exist.
func seedFile(dst string, data []byte) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	return os.WriteFile(dst, data, 0644)
}

// seedFS copies each file from an embedded FS subtree to the data directory,
// skipping files that already exist (preserves user edits).
func seedFS(dataDir string, embedded fs.FS, subdir string) error {
	return fs.WalkDir(embedded, subdir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(embedded, path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		return seedFile(filepath.Join(dataDir, path), data)
	})
}

func touchFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	return f.Close()
}

func initProgress(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	empty := struct {
		Concepts map[string]any `json:"concepts"`
		Sessions int            `json:"sessions"`
	}{Concepts: map[string]any{}, Sessions: 0}
	data, _ := json.Marshal(empty)
	return os.WriteFile(path, append(data, '\n'), 0644)
}
