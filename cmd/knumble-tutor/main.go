package main

import (
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/haflettjm/llm-tutor/internal/app/config"
	llmmcp "github.com/haflettjm/llm-tutor/internal/app/mcp"
	"github.com/haflettjm/llm-tutor/internal/app/setup"
	"github.com/haflettjm/llm-tutor/internal/app/tutor"
	"github.com/haflettjm/llm-tutor/internal/types/event"
	"github.com/haflettjm/llm-tutor/internal/types/profile"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/scratchpad"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("load config", zap.Error(err))
	}

	if err := setup.Run(cfg); err != nil {
		log.Fatal("setup", zap.Error(err))
	}

	prog, err := progress.Load(filepath.Join(cfg.DataDir, "progress.json"))
	if err != nil {
		log.Fatal("load progress", zap.Error(err))
	}

	prof, err := profile.Load(filepath.Join(cfg.DataDir, "learner-profile.json"))
	if err != nil {
		log.Fatal("load learner profile", zap.Error(err))
	}

	scratch, err := scratchpad.Load(filepath.Join(cfg.DataDir, "scratchpad.json"))
	if err != nil {
		log.Fatal("load scratchpad", zap.Error(err))
	}

	evts := event.Open(filepath.Join(cfg.DataDir, "learning-events.jsonl"))

	mcpSrv := llmmcp.New(cfg, prog, prof, scratch, evts)
	go func() {
		if err := mcpSrv.Start(cfg.MCPAddr); err != nil {
			log.Fatal("MCP server", zap.Error(err))
		}
	}()
	log.Info("MCP server started", zap.String("addr", cfg.MCPAddr))

	svc, err := tutor.New(cfg, prog)
	if err != nil {
		log.Fatal("init tutor", zap.Error(err))
	}

	os.Remove(cfg.Socket)
	l, err := net.Listen("unix", cfg.Socket)
	if err != nil {
		log.Fatal("listen", zap.String("socket", cfg.Socket), zap.Error(err))
	}
	defer l.Close()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(ginZap(log))
	r.POST("/tutor", tutorHandler(log, svc))
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })

	log.Info("editor socket listening", zap.String("socket", cfg.Socket))
	if err := http.Serve(l, r); err != nil {
		log.Fatal("serve", zap.Error(err))
	}
}

// tutorHandler returns a Gin handler that routes one editor turn through svc.
// Accepting tutor.Service (not *tutor.Tutor) keeps the HTTP layer decoupled from
// the concrete implementation.
func tutorHandler(log *zap.Logger, svc tutor.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req request.Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := svc.Handle(c.Request.Context(), req)
		if err != nil {
			log.Error("tutor handle", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ginZap logs each request with Zap.
func ginZap(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
		)
	}
}
