package response

// Response is what the backend returns to the editor plugin.
type Response struct {
	Message      string `json:"message"`
	ResponseType string `json:"response_type"` // question | observation | hint | explanation
	ConceptID    string `json:"concept_id,omitempty"`
	HintLevel    int    `json:"hint_level"`
}
