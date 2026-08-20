package main

import (
	"os"

	acpbridge "github.com/haflettjm/llm-tutor/internal/app/acp"
)

func main() {
	socket := os.Getenv("LLM_TUTOR_SOCKET")
	if socket == "" {
		socket = "/tmp/llm-tutor.sock"
	}
	acpbridge.Serve(acpbridge.UnixSocketQuery(socket), os.Stdout, os.Stdin)
}
