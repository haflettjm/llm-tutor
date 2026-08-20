package tutor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haflettjm/llm-tutor/internal/types/progress"
	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
)

func TestLoadSouls(t *testing.T) {
	dir := t.TempDir()
	soulsDir := filepath.Join(dir, "souls")
	if err := os.MkdirAll(soulsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(soulsDir, "concepts-tutor.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	souls, err := loadSouls(soulsDir)
	if err != nil {
		t.Fatalf("loadSouls: %v", err)
	}
	if _, ok := souls["concepts-tutor"]; !ok {
		t.Error("expected concepts-tutor soul to be loaded")
	}
}

func TestSelectSoul(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "progress.json"),
		[]byte(`{"concepts":{},"sessions":0}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	prog, err := progress.Load(filepath.Join(dir, "progress.json"))
	if err != nil {
		t.Fatal(err)
	}

	tut := &Tutor{
		cfg:      typeconfig.Config{DataDir: dir},
		souls:    map[string]string{"concepts-tutor": "# soul content"},
		progress: prog,
	}

	soul := tut.selectSoul()
	if soul == "" {
		t.Error("expected non-empty soul selection")
	}
}
