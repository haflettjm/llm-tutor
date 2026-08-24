package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type LearnerProfile struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	Goal        string `json:"goal"`
	Experience  string `json:"experience"`
	WhyLearning string `json:"why_learning"`

	Context  WorkingContext `json:"context"`
	Style    WorkingStyle   `json:"style"`
	Sessions []SessionNote  `json:"sessions"` // capped at 30
}

type WorkingContext struct {
	Project        string   `json:"project"`
	ProjectGoal    string   `json:"project_goal"`
	CurrentProblem string   `json:"current_problem"`
	TechStack      []string `json:"tech_stack"`
	UpdatedAt      string   `json:"updated_at"`
}

type WorkingStyle struct {
	AvgHintLevel   float64 `json:"avg_hint_level"`
	AvgAttempts    float64 `json:"avg_attempts"`
	PredictionRate float64 `json:"prediction_rate"`

	EffectiveProbes   []string `json:"effective_probes"`
	IneffectiveProbes []string `json:"ineffective_probes"`

	PacingNote        string `json:"pacing_note"`
	FrustrationNote   string `json:"frustration_note"`
	EngagementNote    string `json:"engagement_note"`
	MisconceptionNote string `json:"misconception_note"`
}

type SessionNote struct {
	SessionID string `json:"session_id"`
	Timestamp string `json:"timestamp"`
	ConceptID string `json:"concept_id,omitempty"`
	SoulUsed  string `json:"soul_used"`
	Note      string `json:"note"`
}

// Repo is the interface callers use to read and update the learner profile.
// *Store satisfies this interface; tests can substitute a fake.
type Repo interface {
	Get() LearnerProfile
	UpdateIdentity(goal, experience, whyLearning string) error
	UpdateContext(project, projectGoal, currentProblem string, techStack []string) error
	UpdateStyle(pacingNote, frustrationNote, engagementNote, misconceptionNote string, effectiveProbes, ineffectiveProbes []string, avgHintLevel, avgAttempts, predictionRate float64) error
	AddSessionNote(note SessionNote) error
}

const maxSessionNotes = 30

// Store holds a LearnerProfile in memory and writes to disk on every mutation.
type Store struct {
	mu   sync.RWMutex
	data LearnerProfile
	path string
}

func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var p LearnerProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &Store{data: p, path: path}, nil
}

func (s *Store) Get() LearnerProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

func (s *Store) UpdateIdentity(goal, experience, whyLearning string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if goal != "" {
		s.data.Goal = goal
	}
	if experience != "" {
		s.data.Experience = experience
	}
	if whyLearning != "" {
		s.data.WhyLearning = whyLearning
	}
	s.data.UpdatedAt = now()
	return s.save()
}

func (s *Store) UpdateContext(project, projectGoal, currentProblem string, techStack []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if project != "" {
		s.data.Context.Project = project
	}
	if projectGoal != "" {
		s.data.Context.ProjectGoal = projectGoal
	}
	if currentProblem != "" {
		s.data.Context.CurrentProblem = currentProblem
	}
	if len(techStack) > 0 {
		s.data.Context.TechStack = techStack
	}
	s.data.Context.UpdatedAt = now()
	s.data.UpdatedAt = now()
	return s.save()
}

func (s *Store) UpdateStyle(
	pacingNote, frustrationNote, engagementNote, misconceptionNote string,
	effectiveProbes, ineffectiveProbes []string,
	avgHintLevel, avgAttempts, predictionRate float64,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.data.Style
	if pacingNote != "" {
		st.PacingNote = pacingNote
	}
	if frustrationNote != "" {
		st.FrustrationNote = frustrationNote
	}
	if engagementNote != "" {
		st.EngagementNote = engagementNote
	}
	if misconceptionNote != "" {
		st.MisconceptionNote = misconceptionNote
	}
	if len(effectiveProbes) > 0 {
		st.EffectiveProbes = effectiveProbes
	}
	if len(ineffectiveProbes) > 0 {
		st.IneffectiveProbes = ineffectiveProbes
	}
	if avgHintLevel > 0 {
		st.AvgHintLevel = avgHintLevel
	}
	if avgAttempts > 0 {
		st.AvgAttempts = avgAttempts
	}
	if predictionRate > 0 {
		st.PredictionRate = predictionRate
	}
	s.data.UpdatedAt = now()
	return s.save()
}

func (s *Store) AddSessionNote(note SessionNote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Sessions = append(s.data.Sessions, note)
	if len(s.data.Sessions) > maxSessionNotes {
		s.data.Sessions = s.data.Sessions[len(s.data.Sessions)-maxSessionNotes:]
	}
	s.data.UpdatedAt = now()
	return s.save()
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	return os.Rename(tmp, s.path)
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
