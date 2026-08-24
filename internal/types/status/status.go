// Package status holds the wire types shared between the tutor daemon and the
// editor-facing adapters. They live here rather than in the daemon so the ACP
// bridge can decode them without importing the server.
package status

import "github.com/haflettjm/llm-tutor/internal/types/lesson"

// Progress is what the daemon reports for the "progress" command.
type Progress struct {
	Track        string          `json:"track,omitempty"`
	TrackTitle   string          `json:"track_title,omitempty"`
	Position     int             `json:"position"` // 1-based index of the next concept
	Total        int             `json:"total"`
	Demonstrated int             `json:"demonstrated"`
	Learning     int             `json:"learning"`
	Review       int             `json:"review"`
	Sessions     int             `json:"sessions"`
	ActiveSoul   string          `json:"active_soul,omitempty"`
	Goal         string          `json:"goal,omitempty"`
	NextConcept  *lesson.Concept `json:"next_concept,omitempty"`

	// Focus is what the learner is working on outside any lesson plan.
	Focus string `json:"focus,omitempty"`

	// OffPlan counts concepts recorded that no lesson plan defines -- work the
	// learner brought themselves. A tutor used well accumulates these.
	OffPlan int `json:"off_plan_concepts,omitempty"`

	// Note carries an actionable message when there is nothing to report --
	// no track chosen, or every concept already demonstrated.
	Note string `json:"note,omitempty"`
}

// Plans is what the daemon reports for the "plans" command.
type Plans struct {
	Active string           `json:"active,omitempty"`
	Plans  []lesson.Summary `json:"plans"`
}

// TrackRequest switches the active lesson plan.
type TrackRequest struct {
	Track string `json:"track"`
}

// Health is the daemon's self-report, used by editors to verify the socket is
// serving a healthy daemon rather than a stale file.
type Health struct {
	Status      string `json:"status"`
	Harness     string `json:"harness"`
	ActiveSoul  string `json:"active_soul,omitempty"`
	LessonPlans int    `json:"lesson_plans"`
	DataDir     string `json:"data_dir"`
}

// Error is the body returned for any non-200 response.
type Error struct {
	Error string `json:"error"`
}
