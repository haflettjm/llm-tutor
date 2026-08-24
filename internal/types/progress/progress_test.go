package progress

import (
	"os"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T, seed string) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(path, []byte(seed), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestLoadTolueratesAMissingConceptsMap(t *testing.T) {
	s, _ := newStore(t, `{"current_track":"programming-fundamentals"}`)
	if err := s.SetConceptState("PROG-001", StateLearning); err != nil {
		t.Fatalf("nil concepts map was not initialised: %v", err)
	}
}

func TestSetConceptStateCountsAttempts(t *testing.T) {
	s, _ := newStore(t, `{"concepts":{}}`)
	for i := 0; i < 3; i++ {
		if err := s.SetConceptState("PROG-001", StateLearning); err != nil {
			t.Fatal(err)
		}
	}
	rec := s.Get().Concepts["PROG-001"]
	if rec.Attempts != 3 || rec.State != StateLearning {
		t.Fatalf("record = %+v", rec)
	}
}

func TestDemonstratedOnlyForDemonstratedState(t *testing.T) {
	s, _ := newStore(t, `{"concepts":{}}`)
	for _, state := range []ConceptState{StateNew, StateLearning, StateReview} {
		if err := s.SetConceptState("PROG-001", state); err != nil {
			t.Fatal(err)
		}
		if s.Get().Demonstrated("PROG-001") {
			t.Errorf("state %q counted as demonstrated", state)
		}
	}
	if err := s.SetConceptState("PROG-001", StateDemonstrated); err != nil {
		t.Fatal(err)
	}
	if !s.Get().Demonstrated("PROG-001") {
		t.Error("demonstrated state not recognised")
	}
	if s.Get().Demonstrated("PROG-999") {
		t.Error("an unrecorded concept counted as demonstrated")
	}
}

// Switching tracks must not erase what the learner has already proven.
func TestSetTrackKeepsConceptRecords(t *testing.T) {
	s, _ := newStore(t, `{"concepts":{}}`)
	if err := s.SetConceptState("PROG-001", StateDemonstrated); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTrack("architecture"); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got.CurrentTrack != "architecture" || !got.Demonstrated("PROG-001") {
		t.Fatalf("progress = %+v", got)
	}
}

func TestStartSessionIsIdempotentPerSession(t *testing.T) {
	s, _ := newStore(t, `{"concepts":{}}`)
	for i := 0; i < 5; i++ {
		if err := s.StartSession("s1"); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.Get().Sessions; got != 1 {
		t.Fatalf("sessions = %d, want 1", got)
	}
	if err := s.StartSession("s2"); err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Sessions; got != 2 {
		t.Fatalf("sessions = %d, want 2", got)
	}
}

func TestChangesPersistAtomically(t *testing.T) {
	s, path := newStore(t, `{"concepts":{}}`)
	if err := s.SetTrack("programming-fundamentals"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetConceptState("PROG-001", StateDemonstrated); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Get()
	if got.CurrentTrack != "programming-fundamentals" || !got.Demonstrated("PROG-001") {
		t.Fatalf("state did not survive a reload: %+v", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file was left behind")
	}
}
