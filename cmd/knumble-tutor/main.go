package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/haflettjm/llm-programming-tutor/internal/tutor"
)

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	mentorPath := envOr("TUTOR_MENTOR_PATH", "MENTOR.md")
	soulsPath := envOr("TUTOR_SOULS_PATH", "souls")
	socketPath := envOr("TUTOR_SOCKET", "/tmp/knumble-tutor.sock")

	t, err := tutor.New(apiKey, mentorPath, soulsPath)
	if err != nil {
		log.Fatalf("init tutor: %v", err)
	}

	os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("listen %s: %v", socketPath, err)
	}
	defer l.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /tutor", func(w http.ResponseWriter, r *http.Request) {
		var req tutor.Request
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

	log.Printf("listening on unix://%s", socketPath)
	if err := http.Serve(l, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
