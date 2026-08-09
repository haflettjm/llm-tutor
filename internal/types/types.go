package types

// Harness identifies which AI coding harness routes the tutor's queries.
type Harness string

const (
	HarnessClaudeCode Harness = "claude-code"
	HarnessOpenCode   Harness = "opencode"
	HarnessCodex      Harness = "codex"
	HarnessHermes     Harness = "hermes"
)

// SystemPromptFile returns the filename the harness reads for its injected system prompt.
func (h Harness) SystemPromptFile() string {
	switch h {
	case HarnessClaudeCode:
		return "CLAUDE.md"
	case HarnessOpenCode:
		return "AGENTS.md"
	case HarnessCodex:
		return "CODEX.md"
	case HarnessHermes:
		return "HERMES.md"
	default:
		return "AGENT.md"
	}
}

// Config is loaded from ~/Documents/llm-tutor/config.json.
type Config struct {
	Harness Harness `json:"harness"`            // claude-code | opencode | codex | hermes
	DataDir string  `json:"data_dir,omitempty"` // resolved at runtime, not stored
	Socket  string  `json:"socket,omitempty"`   // unix socket for editor plugin
	MCPAddr string  `json:"mcp_addr,omitempty"` // address our MCP server listens on
}

// Request is what the editor plugin sends on every chat turn.
type Request struct {
	Message   string `json:"message"`
	Diff      string `json:"diff"`
	Language  string `json:"language"`
	ConceptID string `json:"concept_id,omitempty"`
	SessionID string `json:"session_id"`
}

// Response is what the backend returns to the editor plugin.
type Response struct {
	Message      string `json:"message"`
	ResponseType string `json:"response_type"` // question | observation | hint | explanation
	ConceptID    string `json:"concept_id,omitempty"`
	HintLevel    int    `json:"hint_level"`
}

// ConceptState tracks where a learner is with a given concept.
type ConceptState string

const (
	StateNew          ConceptState = "new"
	StateLearning     ConceptState = "learning"
	StateDemonstrated ConceptState = "demonstrated"
	StateReview       ConceptState = "review"
)

// ConceptRecord is one entry in Progress.Concepts.
type ConceptRecord struct {
	State    ConceptState `json:"state"`
	Attempts int          `json:"attempts"`
	LastSeen string       `json:"last_seen,omitempty"`
}

// Progress is the learner's persistent state, stored in progress.json.
type Progress struct {
	LearnerGoal    string                   `json:"learner_goal,omitempty"`
	CurrentTrack   string                   `json:"current_track,omitempty"`
	TrackPosition  int                      `json:"track_position"`
	Concepts       map[string]ConceptRecord `json:"concepts"`
	CurrentProject string                   `json:"current_project,omitempty"`
	Sessions       int                      `json:"sessions"`
	ReviewDue      []string                 `json:"review_due,omitempty"`
}

// Event is one JSON line appended to learning-events.jsonl.
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
