package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/haflettjm/llm-tutor/internal/events"
	"github.com/haflettjm/llm-tutor/internal/progress"
	"github.com/haflettjm/llm-tutor/internal/types"
)

// Server is the MCP server the harness connects to.
// It exposes tools the harness calls to read lesson context and record evidence.
type Server struct {
	mcp      *server.MCPServer
	sse      *server.SSEServer
	progress *progress.Store
	events   *events.Log
	dataDir  string
}

// New creates a configured MCP server backed by the learner's progress and event log.
func New(cfg types.Config, prog *progress.Store, evts *events.Log) *Server {
	s := &Server{
		progress: prog,
		events:   evts,
		dataDir:  cfg.DataDir,
	}

	srv := server.NewMCPServer("llm-tutor", "0.1.0",
		server.WithToolCapabilities(true),
	)

	srv.AddTool(
		mcp.NewTool("get_learner_context",
			mcp.WithDescription("Returns the learner's current track, position, active concept, and concept states."),
		),
		s.handleGetLearnerContext,
	)

	srv.AddTool(
		mcp.NewTool("update_concept_state",
			mcp.WithDescription("Updates the state of a concept for the learner."),
			mcp.WithString("concept_id", mcp.Required(), mcp.Description("Concept identifier e.g. GO-006")),
			mcp.WithString("state", mcp.Required(), mcp.Description("new | learning | demonstrated | review")),
		),
		s.handleUpdateConceptState,
	)

	srv.AddTool(
		mcp.NewTool("append_learning_event",
			mcp.WithDescription("Appends one turn to the append-only learning event log."),
			mcp.WithString("session_id", mcp.Required(), mcp.Description("Session identifier")),
			mcp.WithString("concept_id", mcp.Description("Concept being taught")),
			mcp.WithString("soul_used", mcp.Description("Soul file that was active")),
			mcp.WithString("language", mcp.Description("Programming language in context")),
			mcp.WithString("learner_input", mcp.Required(), mcp.Description("The learner's message")),
			mcp.WithNumber("hint_level", mcp.Description("0 = question, 3 = direct explanation")),
			mcp.WithString("tutor_response_type", mcp.Required(), mcp.Description("question | observation | hint | explanation")),
			mcp.WithString("evidence_observed", mcp.Description("What the learner demonstrated")),
			mcp.WithBoolean("review_flag", mcp.Description("True if this concept should be reviewed soon")),
		),
		s.handleAppendEvent,
	)

	srv.AddTool(
		mcp.NewTool("get_next_concept",
			mcp.WithDescription("Returns the next concept in the active lesson plan that the learner has not yet demonstrated."),
		),
		s.handleGetNextConcept,
	)

	srv.AddTool(
		mcp.NewTool("list_lesson_plans",
			mcp.WithDescription("Lists available lesson plan tracks."),
		),
		s.handleListLessonPlans,
	)

	s.mcp = srv
	s.sse = server.NewSSEServer(srv)
	return s
}

// Start begins serving the MCP SSE endpoint at addr (e.g. ":7890").
func (s *Server) Start(addr string) error {
	return s.sse.Start(addr)
}

func (s *Server) handleGetLearnerContext(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prog := s.progress.Get()
	data, err := json.MarshalIndent(prog, "", "  ")
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
	state := types.ConceptState(stateStr)
	switch state {
	case types.StateNew, types.StateLearning, types.StateDemonstrated, types.StateReview:
	default:
		return nil, fmt.Errorf("invalid state %q -- valid: new, learning, demonstrated, review", stateStr)
	}

	if err := s.progress.SetConceptState(conceptID, state); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(fmt.Sprintf("%s -> %s", conceptID, state)), nil
}

func (s *Server) handleAppendEvent(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	e := types.Event{
		SessionID:    req.GetString("session_id", ""),
		ConceptID:    req.GetString("concept_id", ""),
		SoulUsed:     req.GetString("soul_used", ""),
		Language:     req.GetString("language", ""),
		LearnerInput: req.GetString("learner_input", ""),
		HintLevel:    req.GetInt("hint_level", 0),
		ResponseType: req.GetString("tutor_response_type", ""),
		Evidence:     req.GetString("evidence_observed", ""),
		ReviewFlag:   req.GetBool("review_flag", false),
	}
	if err := s.events.Append(e); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("event recorded"), nil
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
