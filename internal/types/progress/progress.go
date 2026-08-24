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
	Concepts       map[string]ConceptRecord `json:"concepts"`
	CurrentProject string                   `json:"current_project,omitempty"`
	Sessions       int                      `json:"sessions"`
	LastSession    string                   `json:"last_session,omitempty"`

	// Focus is what the learner is working on right now when it is not a
	// concept from the active track -- "goroutine leak in the worker pool",
	// "why this Terraform plan recreates the bucket". The lesson plans are a
	// scaffold, not a cage: most real sessions start from the learner's own
	// work, and that work still deserves to be remembered.
	Focus string `json:"focus,omitempty"`

	// SoulOverride pins the teaching persona regardless of track position, so
	// the tutor can become the debugging coach the moment a bug appears.
	// Cleared by setting it to "".
	SoulOverride string   `json:"soul_override,omitempty"`
	ReviewDue    []string `json:"review_due,omitempty"`
}

// Repo is the interface callers use to read and update learner progress.
// *Store satisfies this interface; tests can substitute a fake.
type Repo interface {
	Get() Progress
	SetConceptState(conceptID string, state ConceptState) error
	SetTrack(track string) error
	StartSession(sessionID string) error
	SetFocus(focus string) error
	SetSoulOverride(soul string) error
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

// SetTrack switches the active lesson plan.
//
// Concept records are kept rather than cleared: a concept already demonstrated
// stays demonstrated even if the learner returns to it from another track.
// Position within a track is derived from those records at read time, never
// stored -- a stored copy can disagree with the concepts it claims to index.
func (s *Store) SetTrack(track string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.CurrentTrack == track {
		return nil
	}
	s.data.CurrentTrack = track
	return s.save()
}

// StartSession records the beginning of a tutoring session. It is idempotent
// per session ID so a reconnecting editor does not inflate the session count.
func (s *Store) StartSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.LastSession == sessionID {
		return nil
	}
	s.data.LastSession = sessionID
	s.data.Sessions++
	return s.save()
}

// SetFocus records what the learner is working on outside the track.
func (s *Store) SetFocus(focus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Focus == focus {
		return nil
	}
	s.data.Focus = focus
	return s.save()
}

// SetSoulOverride pins the teaching persona. Empty clears it and returns
// selection to the active lesson plan.
func (s *Store) SetSoulOverride(soul string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.SoulOverride == soul {
		return nil
	}
	s.data.SoulOverride = soul
	return s.save()
}

// Demonstrated reports whether a concept has been demonstrated. Anything not
// yet recorded counts as not demonstrated.
func (p Progress) Demonstrated(conceptID string) bool {
	return p.Concepts[conceptID].State == StateDemonstrated
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
