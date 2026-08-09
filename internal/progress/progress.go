package progress

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/haflettjm/llm-tutor/internal/types"
)

// Store wraps the learner's Progress with thread-safe read/write and atomic disk persistence.
type Store struct {
	mu   sync.RWMutex
	data types.Progress
	path string
}

// Load reads progress.json from path.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var p types.Progress
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if p.Concepts == nil {
		p.Concepts = make(map[string]types.ConceptRecord)
	}
	return &Store{data: p, path: path}, nil
}

// Get returns a snapshot of the current progress (safe for read).
func (s *Store) Get() types.Progress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// SetConceptState updates the state of one concept and persists to disk.
func (s *Store) SetConceptState(conceptID string, state types.ConceptState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.data.Concepts[conceptID]
	rec.State = state
	rec.Attempts++
	s.data.Concepts[conceptID] = rec
	return s.save()
}

// save writes progress atomically via a temp-file rename (must be called under mu).
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
