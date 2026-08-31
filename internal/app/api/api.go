// Package api builds the HTTP surface the editor adapters talk to over the
// unix socket.
//
// Two kinds of route live here. /tutor routes a turn through the model.
// Everything else answers from local state without spending a model turn --
// "what is my progress" should not cost an API call.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/haflettjm/llm-tutor/internal/app/harness"
	"github.com/haflettjm/llm-tutor/internal/app/tutor"
	typeconfig "github.com/haflettjm/llm-tutor/internal/types/config"
	"github.com/haflettjm/llm-tutor/internal/types/lesson"
	"github.com/haflettjm/llm-tutor/internal/types/profile"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/status"
)

// Deps is everything the routes need. Interfaces rather than concrete stores,
// so the router can be tested without touching disk.
type Deps struct {
	Cfg      typeconfig.Config
	Tutor    tutor.Service
	Progress progress.Repo
	Profile  profile.Repo
	Plans    *lesson.Library
	Log      *zap.Logger

	// ActiveSoul reports the soul composed into the current system prompt.
	// A func rather than a value because it changes as the learner advances.
	ActiveSoul func() string
}

// Router builds the daemon's HTTP handler.
func Router(d Deps) *gin.Engine {
	if d.ActiveSoul == nil {
		d.ActiveSoul = func() string { return "" }
	}
	if d.Log == nil {
		d.Log = zap.NewNop()
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(requestLog(d.Log), gin.Recovery())

	r.POST("/tutor", d.handleTutor)
	r.POST("/tutor/stream", d.handleTutorStream)
	r.GET("/health", d.handleHealth)
	r.GET("/progress", d.handleProgress)
	r.GET("/plans", d.handlePlans)
	r.POST("/track", d.handleSetTrack)
	return r
}

func (d Deps) handleTutor(c *gin.Context) {
	var req request.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, status.Error{Error: err.Error()})
		return
	}
	resp, err := d.Tutor.Handle(c.Request.Context(), req)
	if err != nil {
		d.Log.Error("tutor handle", zap.Error(err))
		c.JSON(http.StatusInternalServerError, status.Error{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (d Deps) handleTutorStream(c *gin.Context) {
	var req request.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, status.Error{Error: err.Error()})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	send := func(event string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	resp, err := d.Tutor.HandleStream(c.Request.Context(), req, func(ch harness.StreamChunk) error {
		if ch.Reset {
			return send("reset", struct{}{})
		}
		return send("chunk", map[string]string{"text": ch.Text})
	})
	if err != nil {
		d.Log.Error("tutor stream", zap.Error(err))
		_ = send("error", status.Error{Error: err.Error()})
		return
	}
	_ = send("done", resp)
}

func (d Deps) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, status.Health{
		Status:      "ok",
		Harness:     string(d.Cfg.Harness),
		ActiveSoul:  d.ActiveSoul(),
		LessonPlans: len(d.Plans.All()),
		DataDir:     d.Cfg.DataDir,
	})
}

func (d Deps) handleProgress(c *gin.Context) {
	c.JSON(http.StatusOK, d.snapshot())
}

// snapshot assembles the progress report from the stores. Exported behaviour is
// tested through the router; kept separate so the assembly is readable.
func (d Deps) snapshot() status.Progress {
	prog := d.Progress.Get()
	prof := d.Profile.Get()

	out := status.Progress{
		Track:      prog.CurrentTrack,
		Sessions:   prog.Sessions,
		ActiveSoul: d.ActiveSoul(),
		Goal:       prof.Goal,
		Focus:      prog.Focus,
	}
	if out.Goal == "" {
		out.Goal = prog.LearnerGoal
	}
	out.OffPlan = d.countOffPlan(prog)

	if prog.CurrentTrack == "" {
		// Not an error state. A learner working through their own code is using
		// this correctly; a lesson plan is optional scaffolding, not a gate.
		out.Note = "No lesson plan selected -- that is fine, the tutor works on whatever you bring it. " +
			"Pick a track only if you want a structured path."
		return out
	}

	plan, err := d.Plans.Plan(prog.CurrentTrack)
	if err != nil {
		out.Note = err.Error()
		return out
	}

	order := plan.Order()
	out.TrackTitle = plan.Title
	out.Total = len(order)

	// A plan whose concept-level entries are still being written has nothing to
	// teach yet. Reporting that as "finished" would be actively misleading.
	if len(order) == 0 {
		out.Note = "this track has no concept-level entries yet -- pick another track, or add concepts to its lesson plan file"
		return out
	}

	for _, id := range order {
		switch prog.Concepts[id].State {
		case progress.StateDemonstrated:
			out.Demonstrated++
		case progress.StateLearning:
			out.Learning++
		case progress.StateReview:
			out.Review++
		}
	}

	next, ok := plan.NextIncomplete(prog.Demonstrated)
	if !ok {
		out.Position = out.Total
		out.Note = "every concept in this track is demonstrated -- pick a new track to keep going"
		return out
	}
	out.NextConcept = &next
	for i, id := range order {
		if id == next.ID {
			out.Position = i + 1
			break
		}
	}
	return out
}

// countOffPlan counts recorded concepts that belong to no lesson plan.
func (d Deps) countOffPlan(prog progress.Progress) int {
	known := make(map[string]bool)
	for _, plan := range d.Plans.All() {
		for _, c := range plan.Concepts {
			known[c.ID] = true
		}
	}
	n := 0
	for id := range prog.Concepts {
		if !known[id] {
			n++
		}
	}
	return n
}

func (d Deps) handlePlans(c *gin.Context) {
	c.JSON(http.StatusOK, status.Plans{
		Active: d.Progress.Get().CurrentTrack,
		Plans:  d.Plans.Summaries(),
	})
}

func (d Deps) handleSetTrack(c *gin.Context) {
	var req status.TrackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, status.Error{Error: err.Error()})
		return
	}
	plan, err := d.Plans.Plan(req.Track)
	if err != nil {
		c.JSON(http.StatusNotFound, status.Error{Error: err.Error()})
		return
	}
	if err := d.Progress.SetTrack(plan.ID); err != nil {
		c.JSON(http.StatusInternalServerError, status.Error{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, d.snapshot())
}

func requestLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
		)
	}
}
