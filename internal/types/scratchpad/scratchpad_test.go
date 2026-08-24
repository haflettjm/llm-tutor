package scratchpad

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newPad(t *testing.T) *Pad {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scratchpad.json")
	if err := os.WriteFile(path, []byte(`{"notes":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAppendRecordsTheSessionItBelongsTo(t *testing.T) {
	p := newPad(t)
	if err := p.Append("s1", 1, "first note"); err != nil {
		t.Fatal(err)
	}
	got := p.Get()
	if got.SessionID != "s1" {
		t.Errorf("session id = %q, want s1", got.SessionID)
	}
	if len(got.Notes) != 1 || got.Notes[0].Content != "first note" {
		t.Errorf("notes = %+v", got.Notes)
	}
	if got.Notes[0].Timestamp == "" {
		t.Error("note has no timestamp")
	}
}

func TestAppendAccumulatesWithinASession(t *testing.T) {
	p := newPad(t)
	for i, note := range []string{"a", "b", "c"} {
		if err := p.Append("s1", i+1, note); err != nil {
			t.Fatal(err)
		}
	}
	if got := p.Get(); len(got.Notes) != 3 || got.Turn != 3 {
		t.Fatalf("notes = %d, turn = %d, want 3/3", len(got.Notes), got.Turn)
	}
}

// The scratchpad is one session's working memory. Interleaving two sessions'
// notes would be worse than dropping the older set.
func TestANewSessionResetsThePad(t *testing.T) {
	p := newPad(t)
	if err := p.Append("s1", 1, "old session note"); err != nil {
		t.Fatal(err)
	}
	if err := p.Append("s2", 1, "new session note"); err != nil {
		t.Fatal(err)
	}
	got := p.Get()
	if got.SessionID != "s2" {
		t.Errorf("session id = %q, want s2", got.SessionID)
	}
	if len(got.Notes) != 1 || got.Notes[0].Content != "new session note" {
		t.Fatalf("notes = %+v, want only the new session's", got.Notes)
	}
}

func TestAppendWithoutASessionIdKeepsTheCurrentPad(t *testing.T) {
	p := newPad(t)
	if err := p.Append("s1", 1, "a"); err != nil {
		t.Fatal(err)
	}
	if err := p.Append("", 2, "b"); err != nil {
		t.Fatal(err)
	}
	got := p.Get()
	if got.SessionID != "s1" || len(got.Notes) != 2 {
		t.Fatalf("pad = %+v", got)
	}
}

func TestAppendPersistsImmediately(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scratchpad.json")
	if err := os.WriteFile(path, []byte(`{"notes":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Append("s1", 1, "durable"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get(); len(got.Notes) != 1 || got.Notes[0].Content != "durable" {
		t.Fatalf("note did not survive a reload: %+v", got)
	}
}

func TestClearEmptiesThePadForANewSession(t *testing.T) {
	p := newPad(t)
	if err := p.Append("s1", 1, "a"); err != nil {
		t.Fatal(err)
	}
	if err := p.Clear("s2"); err != nil {
		t.Fatal(err)
	}
	got := p.Get()
	if len(got.Notes) != 0 || got.SessionID != "s2" {
		t.Fatalf("pad after clear = %+v", got)
	}
}

func TestSavedFileIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scratchpad.json")
	if err := os.WriteFile(path, []byte(`{"notes":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	p, _ := Load(path)
	if err := p.Append("s1", 1, `quotes " and \ backslashes`); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out Scratchpad
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
}
