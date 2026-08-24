package setup

import (
	"os"
	"path/filepath"
	"testing"

	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
)

func cfgIn(dir string) typeconfig.Config {
	return typeconfig.Config{Harness: typeconfig.ClaudeCode, DataDir: dir}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestRunSeedsAFreshDataDir(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(cfgIn(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Created, "MENTOR.md") {
		t.Errorf("MENTOR.md not reported as created: %+v", res)
	}
	for _, rel := range []string{
		"MENTOR.md",
		"souls/concepts-tutor.md",
		"lesson-plans/programming-fundamentals.md",
		"progress.json",
		"learner-profile.json",
		"scratchpad.json",
		"learning-events.jsonl",
		manifestName,
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s was not created: %v", rel, err)
		}
	}
}

func TestRunIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(cfgIn(dir)); err != nil {
		t.Fatal(err)
	}
	res, err := Run(cfgIn(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 0 || len(res.Updated) != 0 || len(res.Kept) != 0 {
		t.Fatalf("second run changed things: %+v", res)
	}
}

// The reason the manifest exists: a learner's edits must survive an upgrade.
func TestLearnerEditsAreNeverOverwritten(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(cfgIn(dir)); err != nil {
		t.Fatal(err)
	}

	mentor := filepath.Join(dir, "MENTOR.md")
	if err := os.WriteFile(mentor, []byte("MY OWN CONTRACT"), 0644); err != nil {
		t.Fatal(err)
	}

	// Pretend the shipped version moved on since we wrote that file.
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	m["MENTOR.md"] = checksum([]byte("an older shipped version"))
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cfgIn(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, mentor); got != "MY OWN CONTRACT" {
		t.Fatalf("learner edit was overwritten: %q", got)
	}
	if !contains(res.Kept, "MENTOR.md") {
		t.Errorf("kept file not reported: %+v", res)
	}
}

// The other half: an untouched file should follow the shipped version.
func TestUntouchedFilesFollowTheShippedVersion(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(cfgIn(dir)); err != nil {
		t.Fatal(err)
	}
	mentor := filepath.Join(dir, "MENTOR.md")
	shipped := read(t, mentor)

	// Simulate an install made by an older build: same on-disk bytes recorded
	// in the manifest, but the file itself is a stale copy.
	stale := "an older shipped MENTOR.md"
	if err := os.WriteFile(mentor, []byte(stale), 0644); err != nil {
		t.Fatal(err)
	}
	m, _ := loadManifest(dir)
	m["MENTOR.md"] = checksum([]byte(stale))
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cfgIn(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, mentor); got != shipped {
		t.Fatalf("untouched file did not update: %q", got)
	}
	if !contains(res.Updated, "MENTOR.md") {
		t.Errorf("update not reported: %+v", res)
	}
}

// Installs predating the manifest still hold the old placeholder scaffolding.
func TestLegacyPlaceholderScaffoldingIsReplaced(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "souls"), 0755); err != nil {
		t.Fatal(err)
	}
	placeholder := "# MENTOR.md\n\n> Placeholder. Complete after Phase 2 manual tutoring sessions.\n"
	if err := os.WriteFile(filepath.Join(dir, "MENTOR.md"), []byte(placeholder), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cfgIn(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "MENTOR.md")); got == placeholder {
		t.Fatal("placeholder scaffolding was kept")
	}
	if !contains(res.Updated, "MENTOR.md") {
		t.Errorf("replacement not reported: %+v", res)
	}
}

// A hand-written file from before the manifest is not scaffolding.
func TestLegacyCustomContentIsKept(t *testing.T) {
	dir := t.TempDir()
	custom := "# My contract\n\nAsk one question per turn.\n"
	if err := os.WriteFile(filepath.Join(dir, "MENTOR.md"), []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(cfgIn(dir)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "MENTOR.md")); got != custom {
		t.Fatalf("pre-manifest custom content was overwritten: %q", got)
	}
}

func TestExistingStateFilesAreNotReset(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(cfgIn(dir)); err != nil {
		t.Fatal(err)
	}
	prog := filepath.Join(dir, "progress.json")
	if err := os.WriteFile(prog, []byte(`{"current_track":"architecture","concepts":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(cfgIn(dir)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, prog); got == "" || !contains([]string{got}, `{"current_track":"architecture","concepts":{}}`) {
		t.Fatalf("progress.json was reset: %q", got)
	}
}

func TestCorruptManifestDoesNotBreakStartup(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(cfgIn(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte("{{{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(cfgIn(dir)); err != nil {
		t.Fatalf("corrupt manifest broke startup: %v", err)
	}
}
