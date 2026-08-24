package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/haflettjm/llm-tutor/internal/app/api"
	"github.com/haflettjm/llm-tutor/internal/app/config"
	llmmcp "github.com/haflettjm/llm-tutor/internal/app/mcp"
	"github.com/haflettjm/llm-tutor/internal/app/setup"
	"github.com/haflettjm/llm-tutor/internal/app/tutor"
	"github.com/haflettjm/llm-tutor/internal/types/event"
	"github.com/haflettjm/llm-tutor/internal/types/lesson"
	"github.com/haflettjm/llm-tutor/internal/types/profile"
	"github.com/haflettjm/llm-tutor/internal/types/progress"
	"github.com/haflettjm/llm-tutor/internal/types/scratchpad"
)

// shutdownGrace bounds how long a clean stop may take before the daemon exits
// anyway. A stuck harness turn must not leave a stale socket behind.
const shutdownGrace = 5 * time.Second

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	if err := run(log); err != nil {
		log.Fatal("fatal", zap.Error(err))
	}
}

func run(log *zap.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	seeded, err := setup.Run(cfg)
	if err != nil {
		return err
	}
	logSeeding(log, seeded)

	prog, err := progress.Load(filepath.Join(cfg.DataDir, "progress.json"))
	if err != nil {
		return err
	}
	prof, err := profile.Load(filepath.Join(cfg.DataDir, "learner-profile.json"))
	if err != nil {
		return err
	}
	scratch, err := scratchpad.Load(filepath.Join(cfg.DataDir, "scratchpad.json"))
	if err != nil {
		return err
	}
	evts := event.Open(filepath.Join(cfg.DataDir, "learning-events.jsonl"))
	plans := lesson.NewLibrary(filepath.Join(cfg.DataDir, "lesson-plans"))

	if n := len(plans.All()); n == 0 {
		log.Warn("no lesson plans parsed -- the tutor will run without a track",
			zap.String("dir", filepath.Join(cfg.DataDir, "lesson-plans")))
	} else {
		log.Info("lesson plans loaded", zap.Int("count", n))
	}

	// The MCP server must be listening before the harness starts, because
	// starting the harness registers this address with it.
	mcpSrv := llmmcp.New(cfg, prog, prof, scratch, evts, plans)
	mcpErr := make(chan error, 1)
	go func() { mcpErr <- mcpSrv.Start(cfg.MCPAddr) }()
	log.Info("MCP server started", zap.String("addr", cfg.MCPAddr))

	svc, err := tutor.New(cfg, prog, plans)
	if err != nil {
		return err
	}
	log.Info("harness ready",
		zap.String("harness", string(cfg.Harness)),
		zap.String("soul", svc.ActiveSoul()))

	l, err := listenSocket(cfg.Socket)
	if err != nil {
		return err
	}

	r := api.Router(api.Deps{
		Cfg:        cfg,
		Tutor:      svc,
		Progress:   prog,
		Profile:    prof,
		Plans:      plans,
		Log:        log,
		ActiveSoul: svc.ActiveSoul,
	})

	httpSrv := &http.Server{Handler: r}
	srvErr := make(chan error, 1)
	go func() { srvErr <- httpSrv.Serve(l) }()
	log.Info("editor socket listening", zap.String("socket", cfg.Socket))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-srvErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("editor socket stopped", zap.Error(err))
		}
	case err := <-mcpErr:
		if err != nil {
			log.Error("MCP server stopped", zap.Error(err))
		}
	}

	shutdown(log, httpSrv, mcpSrv, svc, cfg.Socket)
	return nil
}

// logSeeding surfaces content changes at startup. A learner who customised a
// soul needs to know the shipped version moved on without them.
func logSeeding(log *zap.Logger, r setup.Result) {
	if len(r.Created) > 0 {
		log.Info("content seeded", zap.Strings("files", r.Created))
	}
	if len(r.Updated) > 0 {
		log.Info("content updated to the shipped version", zap.Strings("files", r.Updated))
	}
	if len(r.Kept) > 0 {
		log.Warn("your edits kept; a newer version of these ships with this build",
			zap.Strings("files", r.Kept))
	}
}

// listenSocket binds the unix socket, refusing to clobber one that another
// daemon is already serving. A socket file left behind by a crash is removed;
// a live one is an error, because two daemons sharing state would corrupt it.
func listenSocket(path string) (net.Listener, error) {
	if _, err := os.Stat(path); err == nil {
		if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
			c.Close()
			return nil, errors.New("another knumble-tutor is already listening on " + path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	return net.Listen("unix", path)
}

// shutdown stops accepting turns, then tears down in dependency order: editor
// socket, then harness, then MCP -- so no in-flight turn loses the tools it is
// mid-call on.
func shutdown(log *zap.Logger, httpSrv *http.Server, mcpSrv *llmmcp.Server, svc *tutor.Tutor, socket string) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Warn("editor socket shutdown", zap.Error(err))
	}
	if err := svc.Stop(); err != nil {
		log.Warn("stop harness", zap.Error(err))
	}
	if err := mcpSrv.Shutdown(ctx); err != nil {
		log.Warn("MCP shutdown", zap.Error(err))
	}
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		log.Warn("remove socket", zap.Error(err))
	}
	log.Info("stopped")
}
