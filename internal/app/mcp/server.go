package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/event"
	"github.com/haflettjm/llm-tutor/internal/types/lesson"
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
	plans    *lesson.Library
	dataDir  string
}

// New creates a configured MCP server.
func New(
	cfg typeconfig.Config,
	prog progress.Repo,
	prof profile.Repo,
	scratch scratchpad.Repo,
	evts event.Logger,
	plans *lesson.Library,
) *Server {
	s := &Server{
		progress: prog,
		profile:  prof,
		scratch:  scratch,
		events:   evts,
		plans:    plans,
		dataDir:  cfg.DataDir,
	}

	srv := server.NewMCPServer("llm-tutor", "0.1.0",
		server.WithToolCapabilities(true),
	)

	// ── Session ──────────────────────────────────────────────────────────────

	srv.AddTool(
		mcp.NewTool("start_session",
			mcp.WithDescription(
				"Call this FIRST, before your first reply of a session. Returns a briefing: "+
					"whether this learner is new, what is already known about them, what is NOT known yet, "+
					"what they were doing last time, and where they are in any active lesson plan. "+
					"Follow the session-start protocol in your instructions using what it returns."),
			mcp.WithString("session_id", mcp.Required(), mcp.Description("Session identifier")),
		),
		s.handleStartSession,
	)

	srv.AddTool(
		mcp.NewTool("set_focus",
			mcp.WithDescription(
				"Records what the learner is working on when it is not a lesson-plan concept -- their own bug, "+
					"their own project, a topic they asked about. Call this whenever the subject of the session becomes clear. "+
					"You do NOT need an active lesson plan to teach; the plans are optional scaffolding."),
			mcp.WithString("focus", mcp.Required(), mcp.Description("Short description, e.g. 'goroutine leak in the worker pool'")),
		),
		s.handleSetFocus,
	)

	srv.AddTool(
		mcp.NewTool("set_soul",
			mcp.WithDescription(
				"Switches your teaching persona to match what the learner is actually doing. "+
					"Takes effect from your NEXT turn, so call it as soon as the situation changes, then finish the current turn normally. "+
					"Pass an empty string to return to the lesson plan's choice."),
			mcp.WithString("soul", mcp.Required(), mcp.Description(
				"debugging-coach when they have a bug; code-review when working code should be better; "+
					"concepts-tutor when they need a mental model. Or the name of any file in souls/. Empty string clears the override.")),
		),
		s.handleSetSoul,
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

	srv.AddTool(
		mcp.NewTool("set_lesson_plan",
			mcp.WithDescription("Switches the learner to a different lesson plan track. Use when the learner asks to change topic or when their goal clearly belongs to another track."),
			mcp.WithString("track", mcp.Required(), mcp.Description("Lesson plan id, e.g. programming-fundamentals. Call list_lesson_plans first.")),
		),
		s.handleSetLessonPlan,
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
			mcp.WithString("effective_probes", mcp.Description("Comma-separated probe types that worked. Values only, no prose. One or more of: "+probeVocabulary)),
			mcp.WithString("ineffective_probes", mcp.Description("Comma-separated probe types that did not work. Values only, no prose. One or more of: "+probeVocabulary)),
			mcp.WithNumber("avg_hint_level", mcp.Description("Updated rolling average hint level 0.0-3.0")),
			mcp.WithNumber("avg_attempts", mcp.Description("Updated rolling average attempts per concept")),
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
			mcp.WithString("session_id", mcp.Required(), mcp.Description("Session identifier, so notes are scoped to this session")),
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
// It blocks until the server stops; http.ErrServerClosed after Shutdown is a
// normal stop, not a failure.
func (s *Server) Start(addr string) error {
	if err := s.sse.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the SSE server and disconnects any attached harness.
func (s *Server) Shutdown(ctx context.Context) error {
	s.sse.CloseSessions()
	return s.sse.Shutdown(ctx)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) handleGetLearnerContext(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prog := s.progress.Get()

	type fullContext struct {
		Progress    any             `json:"progress"`
		Profile     any             `json:"profile"`
		Scratch     any             `json:"scratchpad"`
		ActivePlan  *lesson.Summary `json:"active_plan,omitempty"`
		NextConcept *lesson.Concept `json:"next_concept,omitempty"`
		Note        string          `json:"note,omitempty"`
	}
	out := fullContext{
		Progress: prog,
		Profile:  s.profile.Get(),
		Scratch:  s.scratch.Get(),
	}

	if plan, err := s.activePlan(prog.CurrentTrack); err == nil {
		out.ActivePlan = &lesson.Summary{
			ID: plan.ID, Title: plan.Title, Language: plan.Language,
			Goal: plan.Goal, Concepts: len(plan.Concepts),
		}
		if next, ok := plan.NextIncomplete(prog.Demonstrated); ok {
			out.NextConcept = &next
		} else if len(plan.Concepts) == 0 {
			out.Note = "this track has no concept-level entries yet -- call list_lesson_plans and set_lesson_plan to choose one that does"
		} else {
			out.Note = "every concept in this track is demonstrated -- call list_lesson_plans and set_lesson_plan to move on"
		}
	} else {
		out.Note = err.Error()
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}

// handleStartSession assembles the briefing the tutor opens a session with.
//
// The important half is "unknowns": a tutor that does not know it is missing
// the learner's goal will invent a plausible one and teach to it. Naming the
// gaps explicitly is what turns the first turn into a real orientation instead
// of a guess.
func (s *Server) handleStartSession(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := req.RequireString("session_id")
	if err != nil {
		return nil, err
	}
	if err := s.progress.StartSession(sessionID); err != nil {
		return nil, err
	}

	prog := s.progress.Get()
	prof := s.profile.Get()

	type trackBrief struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Position int    `json:"position"`
		Total    int    `json:"total"`
	}
	type briefing struct {
		SessionNumber  int                   `json:"session_number"`
		FirstSession   bool                  `json:"first_session"`
		Goal           string                `json:"goal,omitempty"`
		Experience     string                `json:"experience,omitempty"`
		WhyLearning    string                `json:"why_learning,omitempty"`
		Project        string                `json:"project,omitempty"`
		CurrentProblem string                `json:"current_problem,omitempty"`
		TechStack      []string              `json:"tech_stack,omitempty"`
		Focus          string                `json:"focus,omitempty"`
		Unknowns       []string              `json:"unknowns,omitempty"`
		ActiveTrack    *trackBrief           `json:"active_track,omitempty"`
		NextConcept    *lesson.Concept       `json:"next_concept,omitempty"`
		OffPlanWork    []string              `json:"off_plan_concepts,omitempty"`
		RecentSessions []profile.SessionNote `json:"recent_sessions,omitempty"`
		CarriedNotes   []string              `json:"carried_over_notes,omitempty"`
		Guidance       string                `json:"guidance"`
	}

	b := briefing{
		SessionNumber:  prog.Sessions,
		FirstSession:   prog.Sessions <= 1 && len(prof.Sessions) == 0,
		Goal:           prof.Goal,
		Experience:     prof.Experience,
		WhyLearning:    prof.WhyLearning,
		Project:        prof.Context.Project,
		CurrentProblem: prof.Context.CurrentProblem,
		TechStack:      prof.Context.TechStack,
		Focus:          prog.Focus,
	}

	for label, known := range map[string]bool{
		"what they want to be able to do (goal)": prof.Goal != "",
		"how much experience they have":          prof.Experience != "",
		"why they are learning":                  prof.WhyLearning != "",
		"what they are building":                 prof.Context.Project != "",
		"what languages/tools they use":          len(prof.Context.TechStack) > 0,
	} {
		if !known {
			b.Unknowns = append(b.Unknowns, label)
		}
	}
	sort.Strings(b.Unknowns)

	if plan, err := s.activePlan(prog.CurrentTrack); err == nil && len(plan.Concepts) > 0 {
		order := plan.Order()
		t := trackBrief{ID: plan.ID, Title: plan.Title, Total: len(order)}
		if next, ok := plan.NextIncomplete(prog.Demonstrated); ok {
			t.Position = indexOf(order, next.ID) + 1
			b.NextConcept = &next
		} else {
			t.Position = len(order)
		}
		b.ActiveTrack = &t
	}

	// Concepts worked on that belong to no active plan -- evidence that this
	// learner uses the tutor for their own work, not just the syllabus.
	b.OffPlanWork = s.offPlanConcepts(prog)

	if n := len(prof.Sessions); n > 0 {
		from := n - 3
		if from < 0 {
			from = 0
		}
		b.RecentSessions = prof.Sessions[from:]
	}
	for _, note := range s.scratch.Get().Notes {
		b.CarriedNotes = append(b.CarriedNotes, note.Content)
	}

	if b.FirstSession {
		b.Guidance = "First session. Do not open a lesson plan concept yet. Find out what they want and what they are working on, " +
			"one question at a time, then teach against their own code. Record what you learn with update_learner_identity and update_working_context."
	} else {
		b.Guidance = "Returning learner. Open by reconnecting to what actually happened last time, then ask what they want to work on now. " +
			"Their answer outranks the lesson plan -- if they bring their own problem, teach that and call set_focus."
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}

// offPlanConcepts lists recorded concepts that no lesson plan defines.
func (s *Server) offPlanConcepts(prog progress.Progress) []string {
	known := make(map[string]bool)
	for _, plan := range s.plans.All() {
		for _, c := range plan.Concepts {
			known[c.ID] = true
		}
	}
	var out []string
	for id := range prog.Concepts {
		if !known[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) handleSetFocus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	focus, err := req.RequireString("focus")
	if err != nil {
		return nil, err
	}
	if err := s.progress.SetFocus(focus); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("focus: " + focus), nil
}

func (s *Server) handleSetSoul(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	soul, err := req.RequireString("soul")
	if err != nil {
		return nil, err
	}
	soul = strings.TrimSpace(soul)
	if soul != "" {
		if _, ok := s.souls()[soul]; !ok {
			return mcp.NewToolResultError(fmt.Sprintf(
				"no soul %q -- available: %s", soul, strings.Join(sortedKeys(s.souls()), ", "))), nil
		}
	}
	if err := s.progress.SetSoulOverride(soul); err != nil {
		return nil, err
	}
	if soul == "" {
		return mcp.NewToolResultText("soul override cleared; the lesson plan chooses again from your next turn"), nil
	}
	return mcp.NewToolResultText("teaching as " + soul + " from your next turn"), nil
}

// souls lists the persona files available on disk.
func (s *Server) souls() map[string]bool {
	out := make(map[string]bool)
	matches, err := filepath.Glob(filepath.Join(s.dataDir, "souls", "*.md"))
	if err != nil {
		return out
	}
	for _, m := range matches {
		out[strings.TrimSuffix(filepath.Base(m), ".md")] = true
	}
	return out
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
	plan, err := s.activePlan(prog.CurrentTrack)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if len(plan.Concepts) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf(
			"lesson plan %q has no concept-level entries yet -- call list_lesson_plans and set_lesson_plan to choose a track that does",
			plan.ID,
		)), nil
	}

	next, ok := plan.NextIncomplete(prog.Demonstrated)
	if !ok {
		return mcp.NewToolResultText(fmt.Sprintf(
			"every concept in %q is demonstrated -- call list_lesson_plans and set_lesson_plan to choose a new track",
			plan.ID,
		)), nil
	}

	type nextConcept struct {
		Track    string         `json:"track"`
		Soul     string         `json:"soul,omitempty"`
		Position int            `json:"position"`
		Total    int            `json:"total"`
		Concept  lesson.Concept `json:"concept"`
	}
	payload := nextConcept{
		Track:    plan.ID,
		Soul:     plan.SoulFor(next.ID),
		Position: indexOf(plan.Order(), next.ID) + 1,
		Total:    len(plan.Concepts),
		Concept:  next,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleListLessonPlans(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	summaries := s.plans.Summaries()
	if len(summaries) == 0 {
		return mcp.NewToolResultError("no lesson plans found in " + s.dataDir + "/lesson-plans"), nil
	}
	data, err := json.MarshalIndent(struct {
		Active string           `json:"active_track,omitempty"`
		Plans  []lesson.Summary `json:"plans"`
	}{
		Active: s.progress.Get().CurrentTrack,
		Plans:  summaries,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleSetLessonPlan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	track, err := req.RequireString("track")
	if err != nil {
		return nil, err
	}
	plan, err := s.plans.Plan(track)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.progress.SetTrack(plan.ID); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(fmt.Sprintf("active track is now %s (%s), %d concepts", plan.ID, plan.Title, len(plan.Concepts))), nil
}

// activePlan resolves the learner's current track, turning both "no track set"
// and "track names a plan that no longer exists" into an actionable message the
// tutor can relay rather than an opaque failure.
func (s *Server) activePlan(track string) (*lesson.Plan, error) {
	if track == "" {
		return nil, fmt.Errorf("no active lesson plan -- call list_lesson_plans then set_lesson_plan to choose one")
	}
	return s.plans.Plan(track)
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
	effective := parseProbes(req.GetString("effective_probes", ""))
	ineffective := parseProbes(req.GetString("ineffective_probes", ""))

	err := s.profile.UpdateStyle(
		req.GetString("pacing_note", ""),
		req.GetString("frustration_note", ""),
		req.GetString("engagement_note", ""),
		req.GetString("misconception_note", ""),
		effective,
		ineffective,
		req.GetFloat("avg_hint_level", 0),
		req.GetFloat("avg_attempts", 0),
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
	if err := s.scratch.Append(req.GetString("session_id", ""), req.GetInt("turn", 0), note); err != nil {
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

// splitComma splits a free-form comma-separated list. Used for tech stacks,
// where the values are open-ended by nature. Probe types are not open-ended --
// see parseProbes.
func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// probeTypes is the closed vocabulary for the probe fields on a learner
// profile. These are lists meant to be counted and compared across sessions, so
// they have to stay values rather than prose.
var probeTypes = map[string]bool{
	"prediction":      true,
	"variation":       true,
	"teach-back":      true,
	"trace-table":     true,
	"analogy":         true,
	"counter-example": true,
	"decomposition":   true,
	"transfer":        true,
}

// probeVocabulary lists the accepted values for the tool description, so the
// vocabulary is defined in exactly one place.
var probeVocabulary = strings.Join(sortedKeys(probeTypes), ", ")

// parseProbes splits a comma-separated probe list and keeps only recognised
// values.
//
// The filter is load-bearing, not defensive decoration: a model handed a field
// described as "comma-separated" will sometimes write a sentence into it, and
// splitting that on commas turns one observation into several fragments that
// then persist in the profile as if they were probe names.
func parseProbes(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		v := strings.ToLower(strings.TrimSpace(part))
		if probeTypes[v] {
			out = append(out, v)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

func timeNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
