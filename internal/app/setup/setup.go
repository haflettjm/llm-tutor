package setup

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/haflettjm/llm-tutor/internal/app/content"
	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/profile"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/scratchpad"
)

// Run creates the data directory, seeds default content files, and initializes
// state files. Existing user-edited files are never overwritten.
func Run(cfg typeconfig.Config) error {
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
	if err := initProgress(filepath.Join(cfg.DataDir, "progress.json")); err != nil {
		return err
	}
	if err := initProfile(filepath.Join(cfg.DataDir, "learner-profile.json")); err != nil {
		return err
	}
	return initScratchpad(filepath.Join(cfg.DataDir, "scratchpad.json"))
}

func seedFile(dst string, data []byte) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	return os.WriteFile(dst, data, 0644)
}

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
	empty := progress.Progress{
		Concepts: make(map[string]progress.ConceptRecord),
	}
	data, _ := json.MarshalIndent(empty, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func initProfile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	empty := profile.LearnerProfile{
		ID:        "local",
		CreatedAt: ts,
		UpdatedAt: ts,
		Context: profile.WorkingContext{
			TechStack: []string{},
			UpdatedAt: ts,
		},
		Style: profile.WorkingStyle{
			EffectiveProbes:   []string{},
			IneffectiveProbes: []string{},
		},
		Sessions: []profile.SessionNote{},
	}
	data, _ := json.MarshalIndent(empty, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func initScratchpad(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	empty := scratchpad.Scratchpad{
		StartedAt: ts,
		UpdatedAt: ts,
		Notes:     []scratchpad.Note{},
	}
	data, _ := json.MarshalIndent(empty, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}
