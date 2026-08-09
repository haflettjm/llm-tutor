package main

import (
	stdlog "log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"encoding/json"

	"github.com/haflettjm/llm-tutor/internal/config"
	"github.com/haflettjm/llm-tutor/internal/events"
	llmmcp "github.com/haflettjm/llm-tutor/internal/mcp"
	"github.com/haflettjm/llm-tutor/internal/progress"
	"github.com/haflettjm/llm-tutor/internal/setup"
	"github.com/haflettjm/llm-tutor/internal/tutor"
	"github.com/haflettjm/llm-tutor/internal/types"
)

func main() {
	// 1. Load config from ~/Documents/llm-tutor/config.json.
	cfg, err := config.Load()
	if err != nil {
		stdlog.Fatal(err)
	}

	// 2. Create data directory, seed default content, initialize state files.
	if err := setup.Run(cfg); err != nil {
		stdlog.Fatalf("setup: %v", err)
	}

	// 3. Load persistent learner state.
	prog, err := progress.Load(filepath.Join(cfg.DataDir, "progress.json"))
	if err != nil {
		stdlog.Fatalf("load progress: %v", err)
	}

	evts := events.Open(filepath.Join(cfg.DataDir, "learning-events.jsonl"))

	// 4. Start the MCP server the harness will call back into.
	mcpSrv := llmmcp.New(cfg, prog, evts)
	go func() {
		if err := mcpSrv.Start(cfg.MCPAddr); err != nil {
			stdlog.Fatalf("MCP server: %v", err)
		}
	}()
	stdlog.Printf("MCP server listening on %s", cfg.MCPAddr)

	// 5. Initialize tutor: load MENTOR.md + souls, inject system prompt, start harness.
	t, err := tutor.New(cfg, prog)
	if err != nil {
		stdlog.Fatalf("init tutor: %v", err)
	}

	// 6. Start the editor plugin socket.
	os.Remove(cfg.Socket)
	l, err := net.Listen("unix", cfg.Socket)
	if err != nil {
		stdlog.Fatalf("listen %s: %v", cfg.Socket, err)
	}
	defer l.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /tutor", func(w http.ResponseWriter, r *http.Request) {
		var req types.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resp, err := t.Handle(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	stdlog.Printf("editor socket listening on unix://%s", cfg.Socket)
	if err := http.Serve(l, mux); err != nil {
		stdlog.Fatalf("serve: %v", err)
	}
}
