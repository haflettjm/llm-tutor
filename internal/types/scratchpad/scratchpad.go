package scratchpad

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type Scratchpad struct {
	SessionID string `json:"session_id"`
	StartedAt string `json:"started_at"`
	UpdatedAt string `json:"updated_at"`
	Turn      int    `json:"turn"`
	Notes     []Note `json:"notes"`
}

type Note struct {
	Turn      int    `json:"turn"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
}

// Repo is the interface callers use to read and write the session scratchpad.
// *Pad satisfies this interface; tests can substitute a fake.
type Repo interface {
	Get() Scratchpad
	Append(sessionID string, turn int, content string) error
	Clear(sessionID string) error
}

// Pad holds the in-session scratchpad in memory and writes to disk on every append.
type Pad struct {
	mu   sync.Mutex
	data Scratchpad
	path string
}

func Load(path string) (*Pad, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s Scratchpad
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &Pad{data: s, path: path}, nil
}

func (p *Pad) Get() Scratchpad {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.data
}

// Append records a note. A note arriving for a different session than the pad
// currently holds starts a fresh pad: the scratchpad is the tutor's working
// memory for one session, and mixing two sessions' notes is worse than losing
// the older set, which the session note already summarises.
func (p *Pad) Append(sessionID string, turn int, content string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if sessionID != "" && p.data.SessionID != sessionID {
		now := time.Now().UTC().Format(time.RFC3339)
		p.data = Scratchpad{SessionID: sessionID, StartedAt: now, UpdatedAt: now, Notes: []Note{}}
	}
	p.data.Turn = turn
	p.data.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	p.data.Notes = append(p.data.Notes, Note{
		Turn:      turn,
		Timestamp: p.data.UpdatedAt,
		Content:   content,
	})
	return p.save()
}

func (p *Pad) Clear(sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = Scratchpad{
		SessionID: sessionID,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Notes:     []Note{},
	}
	return p.save()
}

func (p *Pad) save() error {
	data, err := json.MarshalIndent(p.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal scratchpad: %w", err)
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	return os.Rename(tmp, p.path)
}
