package events

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/haflettjm/llm-tutor/internal/types"
)

// Log appends learning events to an append-only JSONL file.
type Log struct {
	path string
}

// Open returns a Log that appends to path.
func Open(path string) *Log {
	return &Log{path: path}
}

// Append writes one event as a JSON line. Timestamp is set to now if empty.
func (l *Log) Append(e types.Event) error {
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
