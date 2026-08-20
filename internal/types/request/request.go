package request

// Request is what the editor plugin sends on every chat turn.
type Request struct {
	Message   string `json:"message"`
	Diff      string `json:"diff"`
	Language  string `json:"language"`
	ConceptID string `json:"concept_id,omitempty"`
	SessionID string `json:"session_id"`
}
