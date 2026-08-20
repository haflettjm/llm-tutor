package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/event"
	"github.com/haflettjm/llm-tutor/internal/types/profile"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/scratchpad"
)

// Server is the MCP server the harness connects to.
type Server struct {
	mcp      *server.MCPServer
	sse      *server.SSEServer
	progress progress.Repo
	profile  profile.Repo
	scratch  scratchpad.Repo
	events   event.Logger
	dataDir  string
}

// New creates a configured MCP server.
func New(
	cfg typeconfig.Config,
	prog progress.Repo,
	prof profile.Repo,
	scratch scratchpad.Repo,
	evts event.Logger,
) *Server {
	s := &Server{
		progress: prog,
		profile:  prof,
		scratch:  scratch,
		events:   evts,
		dataDir:  cfg.DataDir,
	}

	srv := server.NewMCPServer("llm-tutor", "0.1.0",
		server.WithToolCapabilities(true),
	)

	// ── Context ──────────────────────────────────────────────────────────────

	srv.AddTool(
		mcp.NewTool("get_learner_context",
			mcp.WithDescription("Returns the full learner context: progress, profile, working style, and current scratchpad. Call this at the start of every session."),
		),
		s.handleGetLearnerContext,
	)

	// ── Progress ─────────────────────────────────────────────────────────────

	srv.AddTool(
		mcp.NewTool("update_concept_state",
			mcp.WithDescription("Updates the state of a concept. Call after every meaningful learner attempt."),
			mcp.WithString("concept_id", mcp.Required(), mcp.Description("e.g. PROG-004")),
			mcp.WithString("state", mcp.Required(), mcp.Description("new | learning | demonstrated | review")),
		),
		s.handleUpdateConceptState,
	)

	srv.AddTool(
		mcp.NewTool("get_next_concept",
			mcp.WithDescription("Returns the next concept in the active lesson plan the learner has not yet demonstrated."),
		),
		s.handleGetNextConcept,
	)

	srv.AddTool(
		mcp.NewTool("list_lesson_plans",
			mcp.WithDescription("Lists available lesson plan tracks."),
		),
		s.handleListLessonPlans,
	)

	// ── Learner profile ───────────────────────────────────────────────────────

	srv.AddTool(
		mcp.NewTool("update_learner_identity",
			mcp.WithDescription("Updates who the learner is and why they are here. Call when they share new information about themselves."),
			mcp.WithString("goal", mcp.Description("What they want to achieve")),
			mcp.WithString("experience", mcp.Description("Their background: 'one week', 'some Python before'")),
			mcp.WithString("why_learning", mcp.Description("Their motivation")),
		),
		s.handleUpdateLearnerIdentity,
	)

	srv.AddTool(
		mcp.NewTool("update_working_context",
			mcp.WithDescription("Updates what the learner is actively building. Call whenever their project context becomes clearer from the diff or conversation."),
			mcp.WithString("project", mcp.Description("What they are building")),
			mcp.WithString("project_goal", mcp.Description("Why they are building it")),
			mcp.WithString("current_problem", mcp.Description("What is blocking them right now")),
			mcp.WithString("tech_stack", mcp.Description("Comma-separated list of languages/libraries observed from diffs")),
		),
		s.handleUpdateWorkingContext,
	)

	srv.AddTool(
		mcp.NewTool("update_working_style",
			mcp.WithDescription("Updates the tutor's observations about how this learner learns. Call at session end or after a significant pattern is confirmed."),
			mcp.WithString("pacing_note", mcp.Description("How fast/slow they move through material")),
			mcp.WithString("frustration_note", mcp.Description("How they respond when stuck")),
			mcp.WithString("engagement_note", mcp.Description("What makes them most engaged")),
			mcp.WithString("misconception_note", mcp.Description("Recurring misconceptions observed")),
			mcp.WithString("effective_probes", mcp.Description("Comma-separated probe types that worked: prediction, variation, teach-back, trace-table")),
			mcp.WithString("ineffective_probes", mcp.Description("Comma-separated probe types that did not work")),
			mcp.WithNumber("avg_hint_level", mcp.Description("Updated rolling average hint level 0.0-3.0")),
			mcp.WithNumber("prediction_rate", mcp.Description("Updated fraction of turns where learner predicted before running 0.0-1.0")),
		),
		s.handleUpdateWorkingStyle,
	)

	// ── Scratchpad ────────────────────────────────────────────────────────────

	srv.AddTool(
		mcp.NewTool("write_scratchpad",
			mcp.WithDescription("Write a note to your working scratchpad. Use freely between turns to record observations, hypotheses, what to try next, or analogies that landed. Persisted immediately."),
			mcp.WithString("note", mcp.Required(), mcp.Description("Your observation or plan in plain prose")),
			mcp.WithNumber("turn", mcp.Required(), mcp.Description("Current conversation turn number")),
		),
		s.handleWriteScratchpad,
	)

	srv.AddTool(
		mcp.NewTool("end_session",
			mcp.WithDescription("Close the current session: write a SessionNote summarizing what happened, update working style stats, and clear the scratchpad for the next session."),
			mcp.WithString("session_id", mcp.Required(), mcp.Description("Session identifier")),
			mcp.WithString("concept_id", mcp.Description("Primary concept worked on")),
			mcp.WithString("soul_used", mcp.Description("Soul file active this session")),
			mcp.WithString("note", mcp.Required(), mcp.Description("2-3 sentence summary of what was observed and what happened")),
		),
		s.handleEndSession,
	)

	// ── Event log ────────────────────────────────────────────────────────────

	srv.AddTool(
		mcp.NewTool("append_learning_event",
			mcp.WithDescription("Appends one turn to the append-only learning event log."),
			mcp.WithString("session_id", mcp.Required(), mcp.Description("Session identifier")),
			mcp.WithString("concept_id", mcp.Description("Concept being taught")),
			mcp.WithString("soul_used", mcp.Description("Soul file that was active")),
			mcp.WithString("language", mcp.Description("Programming language in context")),
			mcp.WithString("learner_input", mcp.Required(), mcp.Description("The learner's message")),
			mcp.WithNumber("hint_level", mcp.Description("0 = question only, 3 = direct explanation")),
			mcp.WithString("tutor_response_type", mcp.Required(), mcp.Description("question | observation | hint | explanation")),
			mcp.WithString("evidence_observed", mcp.Description("What the learner demonstrated")),
			mcp.WithBoolean("review_flag", mcp.Description("True if this concept should be reviewed soon")),
		),
		s.handleAppendEvent,
	)

	s.mcp = srv
	s.sse = server.NewSSEServer(srv)
	return s
}

// Start begins serving the MCP SSE endpoint at addr (e.g. ":7890").
func (s *Server) Start(addr string) error {
	return s.sse.Start(addr)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) handleGetLearnerContext(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	type fullContext struct {
		Progress interface{} `json:"progress"`
		Profile  interface{} `json:"profile"`
		Scratch  interface{} `json:"scratchpad"`
	}
	ctx := fullContext{
		Progress: s.progress.Get(),
		Profile:  s.profile.Get(),
		Scratch:  s.scratch.Get(),
	}
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleUpdateConceptState(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	conceptID, err := req.RequireString("concept_id")
	if err != nil {
		return nil, err
	}
	stateStr, err := req.RequireString("state")
	if err != nil {
		return nil, err
	}
	state := progress.ConceptState(stateStr)
	switch state {
	case progress.StateNew, progress.StateLearning, progress.StateDemonstrated, progress.StateReview:
	default:
		return nil, fmt.Errorf("invalid state %q -- valid: new, learning, demonstrated, review", stateStr)
	}
	if err := s.progress.SetConceptState(conceptID, state); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(fmt.Sprintf("%s -> %s", conceptID, state)), nil
}

func (s *Server) handleGetNextConcept(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prog := s.progress.Get()
	if prog.CurrentTrack == "" {
		return mcp.NewToolResultText("no active lesson plan -- set current_track in progress.json"), nil
	}
	planPath := filepath.Join(s.dataDir, "lesson-plans", prog.CurrentTrack+".md")
	return mcp.NewToolResultText(fmt.Sprintf(
		"track: %s, position: %d, plan: %s -- full concept detail not yet parsed from plan file",
		prog.CurrentTrack, prog.TrackPosition, planPath,
	)), nil
}

func (s *Server) handleListLessonPlans(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pattern := filepath.Join(s.dataDir, "lesson-plans", "*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(matches)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleUpdateLearnerIdentity(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.profile.UpdateIdentity(
		req.GetString("goal", ""),
		req.GetString("experience", ""),
		req.GetString("why_learning", ""),
	); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("learner identity updated"), nil
}

func (s *Server) handleUpdateWorkingContext(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var techStack []string
	if ts := req.GetString("tech_stack", ""); ts != "" {
		techStack = splitComma(ts)
	}
	if err := s.profile.UpdateContext(
		req.GetString("project", ""),
		req.GetString("project_goal", ""),
		req.GetString("current_problem", ""),
		techStack,
	); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("working context updated"), nil
}

func (s *Server) handleUpdateWorkingStyle(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var effective, ineffective []string
	if v := req.GetString("effective_probes", ""); v != "" {
		effective = splitComma(v)
	}
	if v := req.GetString("ineffective_probes", ""); v != "" {
		ineffective = splitComma(v)
	}
	err := s.profile.UpdateStyle(
		req.GetString("pacing_note", ""),
		req.GetString("frustration_note", ""),
		req.GetString("engagement_note", ""),
		req.GetString("misconception_note", ""),
		effective,
		ineffective,
		req.GetFloat("avg_hint_level", 0),
		0, // avg_attempts not yet wired
		req.GetFloat("prediction_rate", 0),
	)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("working style updated"), nil
}

func (s *Server) handleWriteScratchpad(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	note, err := req.RequireString("note")
	if err != nil {
		return nil, err
	}
	if err := s.scratch.Append(req.GetInt("turn", 0), note); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("note written"), nil
}

func (s *Server) handleEndSession(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := req.RequireString("session_id")
	if err != nil {
		return nil, err
	}
	note, err := req.RequireString("note")
	if err != nil {
		return nil, err
	}
	if err := s.profile.AddSessionNote(profile.SessionNote{
		SessionID: sessionID,
		Timestamp: timeNow(),
		ConceptID: req.GetString("concept_id", ""),
		SoulUsed:  req.GetString("soul_used", ""),
		Note:      note,
	}); err != nil {
		return nil, err
	}
	if err := s.scratch.Clear(sessionID); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("session ended, scratchpad cleared"), nil
}

func (s *Server) handleAppendEvent(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.events.Append(event.Event{
		SessionID:    req.GetString("session_id", ""),
		ConceptID:    req.GetString("concept_id", ""),
		SoulUsed:     req.GetString("soul_used", ""),
		Language:     req.GetString("language", ""),
		LearnerInput: req.GetString("learner_input", ""),
		HintLevel:    req.GetInt("hint_level", 0),
		ResponseType: req.GetString("tutor_response_type", ""),
		Evidence:     req.GetString("evidence_observed", ""),
		ReviewFlag:   req.GetBool("review_flag", false),
	}); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("event recorded"), nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func splitComma(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == ',' {
			if t := trimSpace(s[start:i]); t != "" {
				out = append(out, t)
			}
			start = i + 1
		}
	}
	if t := trimSpace(s[start:]); t != "" {
		out = append(out, t)
	}
	return out
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func timeNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
