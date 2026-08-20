package progress

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type ConceptState string

const (
	StateNew          ConceptState = "new"
	StateLearning     ConceptState = "learning"
	StateDemonstrated ConceptState = "demonstrated"
	StateReview       ConceptState = "review"
)

type ConceptRecord struct {
	State    ConceptState `json:"state"`
	Attempts int          `json:"attempts"`
	LastSeen string       `json:"last_seen,omitempty"`
}

type Progress struct {
	LearnerGoal    string                   `json:"learner_goal,omitempty"`
	CurrentTrack   string                   `json:"current_track,omitempty"`
	TrackPosition  int                      `json:"track_position"`
	Concepts       map[string]ConceptRecord `json:"concepts"`
	CurrentProject string                   `json:"current_project,omitempty"`
	Sessions       int                      `json:"sessions"`
	ReviewDue      []string                 `json:"review_due,omitempty"`
}

// Repo is the interface callers use to read and update learner progress.
// *Store satisfies this interface; tests can substitute a fake.
type Repo interface {
	Get() Progress
	SetConceptState(conceptID string, state ConceptState) error
}

// Store wraps Progress with thread-safe read/write and atomic disk persistence.
type Store struct {
	mu   sync.RWMutex
	data Progress
	path string
}

func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var p Progress
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if p.Concepts == nil {
		p.Concepts = make(map[string]ConceptRecord)
	}
	return &Store{data: p, path: path}, nil
}

func (s *Store) Get() Progress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

func (s *Store) SetConceptState(conceptID string, state ConceptState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.data.Concepts[conceptID]
	rec.State = state
	rec.Attempts++
	s.data.Concepts[conceptID] = rec
	return s.save()
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	return os.Rename(tmp, s.path)
}
