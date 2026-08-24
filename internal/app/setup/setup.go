// Package setup prepares the learner's data directory on every daemon start.
package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/haflettjm/llm-tutor/internal/app/content"
	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/profile"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/scratchpad"
)

// manifestName records the checksum of every content file as this daemon last
// wrote it. It is what lets a later version ship an improved MENTOR.md without
// either silently discarding the learner's edits or stranding them on the
// version they first installed.
const manifestName = ".seeded.json"

// Result reports what Run changed, so the daemon can say so at startup.
type Result struct {
	Created []string // did not exist
	Updated []string // shipped version changed and the local copy was untouched
	Kept    []string // shipped version changed but the learner had edited it
}

// Run creates the data directory, seeds content files, and initializes state
// files. Learner-edited content is never overwritten.
func Run(cfg typeconfig.Config) (Result, error) {
	var res Result

	for _, d := range []string{
		cfg.DataDir,
		filepath.Join(cfg.DataDir, "souls"),
		filepath.Join(cfg.DataDir, "lesson-plans"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return res, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	m, err := loadManifest(cfg.DataDir)
	if err != nil {
		return res, err
	}

	if err := seedFile(cfg.DataDir, m, &res, "MENTOR.md", content.MentorMD); err != nil {
		return res, err
	}
	if err := seedFS(cfg.DataDir, m, &res, content.SoulsFS, "souls"); err != nil {
		return res, err
	}
	if err := seedFS(cfg.DataDir, m, &res, content.LessonPlansFS, "lesson-plans"); err != nil {
		return res, err
	}
	if err := saveManifest(cfg.DataDir, m); err != nil {
		return res, err
	}

	if err := touchFile(filepath.Join(cfg.DataDir, "learning-events.jsonl")); err != nil {
		return res, err
	}
	if err := initProgress(filepath.Join(cfg.DataDir, "progress.json")); err != nil {
		return res, err
	}
	if err := initProfile(filepath.Join(cfg.DataDir, "learner-profile.json")); err != nil {
		return res, err
	}
	return res, initScratchpad(filepath.Join(cfg.DataDir, "scratchpad.json"))
}

// seedFile writes shipped content to rel unless the learner has customised it.
//
// Four cases:
//   - absent            -> write it
//   - identical         -> nothing to do, just record the checksum
//   - matches manifest  -> the learner never touched it, so take the new version
//   - anything else     -> their edit wins; report it and move on
func seedFile(dataDir string, m map[string]string, res *Result, rel string, data []byte) error {
	dst := filepath.Join(dataDir, rel)
	want := checksum(data)

	cur, err := os.ReadFile(dst)
	if os.IsNotExist(err) {
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		m[rel] = want
		res.Created = append(res.Created, rel)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dst, err)
	}

	got := checksum(cur)
	if got == want {
		m[rel] = want
		return nil
	}

	recorded, tracked := m[rel]
	if (tracked && recorded == got) || (!tracked && isScaffold(cur)) {
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		m[rel] = want
		res.Updated = append(res.Updated, rel)
		return nil
	}

	res.Kept = append(res.Kept, rel)
	return nil
}

// isScaffold recognises the unfinished placeholder content earlier versions
// shipped. Those files were never meant to be kept, and a real MENTOR.md or
// soul will not open a blockquote by announcing itself as a placeholder.
//
// It only applies to installs predating the manifest; afterwards the recorded
// checksum answers the question exactly.
func isScaffold(data []byte) bool {
	head := string(data)
	if len(head) > 2048 {
		head = head[:2048]
	}
	return strings.Contains(head, "> Placeholder.") || strings.Contains(head, "> Stub.")
}

func seedFS(dataDir string, m map[string]string, res *Result, embedded fs.FS, subdir string) error {
	return fs.WalkDir(embedded, subdir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(embedded, path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		return seedFile(dataDir, m, res, filepath.ToSlash(path), data)
	})
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadManifest(dataDir string) (map[string]string, error) {
	m := make(map[string]string)
	data, err := os.ReadFile(filepath.Join(dataDir, manifestName))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", manifestName, err)
	}
	// A corrupt manifest must not stop the daemon: an empty one falls back to
	// the conservative "keep whatever is on disk" path.
	_ = json.Unmarshal(data, &m)
	return m, nil
}

func saveManifest(dataDir string, m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir, manifestName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
