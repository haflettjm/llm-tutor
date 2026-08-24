package lesson

import (
	"fmt"
	"sync"
)

// NextIncomplete returns the first concept in teaching order that done reports
// false for. done is supplied by the caller so this package stays free of any
// dependency on how progress is stored.
func (p *Plan) NextIncomplete(done func(conceptID string) bool) (Concept, bool) {
	for _, id := range p.Order() {
		if done(id) {
			continue
		}
		if c, ok := p.Concept(id); ok {
			return c, true
		}
	}
	return Concept{}, false
}

// Library holds the parsed lesson plans found in a directory.
//
// Plans are re-read from disk on every access. They are a handful of small
// markdown files that the learner is expected to edit by hand while the daemon
// runs, so picking up an edit without a restart matters more than the read cost.
type Library struct {
	dir string

	mu     sync.Mutex
	cached map[string]*Plan
}

// NewLibrary returns a Library reading from dir.
func NewLibrary(dir string) *Library {
	return &Library{dir: dir}
}

// All returns every plan keyed by ID, re-reading the directory. On a read error
// the previously loaded plans are returned so a transient failure does not blank
// the learner's track mid-session.
func (l *Library) All() map[string]*Plan {
	l.mu.Lock()
	defer l.mu.Unlock()

	plans, err := LoadDir(l.dir)
	if err != nil || len(plans) == 0 {
		if l.cached != nil {
			return l.cached
		}
		if err != nil {
			return map[string]*Plan{}
		}
	}
	l.cached = plans
	return plans
}

// Plan returns the plan with the given ID.
func (l *Library) Plan(id string) (*Plan, error) {
	plans := l.All()
	p, ok := plans[id]
	if !ok {
		return nil, fmt.Errorf("no lesson plan %q -- available: %v", id, IDs(plans))
	}
	return p, nil
}

// Summary is the compact shape returned when listing available tracks.
type Summary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Language string `json:"language,omitempty"`
	Goal     string `json:"goal,omitempty"`
	Concepts int    `json:"concepts"`
}

// Summaries lists every plan in stable ID order.
func (l *Library) Summaries() []Summary {
	plans := l.All()
	ids := IDs(plans)
	out := make([]Summary, 0, len(ids))
	for _, id := range ids {
		p := plans[id]
		out = append(out, Summary{
			ID:       p.ID,
			Title:    p.Title,
			Language: p.Language,
			Goal:     p.Goal,
			Concepts: len(p.Concepts),
		})
	}
	return out
}
