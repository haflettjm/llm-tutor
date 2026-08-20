package event

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Event struct {
	Timestamp    string `json:"ts"`
	SessionID    string `json:"session_id"`
	ConceptID    string `json:"concept_id,omitempty"`
	SoulUsed     string `json:"soul_used,omitempty"`
	Language     string `json:"language,omitempty"`
	LearnerInput string `json:"learner_input"`
	HintLevel    int    `json:"hint_level"`
	ResponseType string `json:"tutor_response_type"`
	Evidence     string `json:"evidence_observed,omitempty"`
	ReviewFlag   bool   `json:"review_flag"`
}

// Logger is the interface callers use to record learning events.
// *Log satisfies this interface; tests can substitute a fake.
type Logger interface {
	Append(e Event) error
}

// Log appends learning events to an append-only JSONL file.
type Log struct {
	path string
}

func Open(path string) *Log {
	return &Log{path: path}
}

func (l *Log) Append(e Event) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("open %s: %w", l.path, err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}
