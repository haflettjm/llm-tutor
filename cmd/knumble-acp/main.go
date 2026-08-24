// Command knumble-acp is the editor-facing ACP adapter. Editors spawn it per
// workspace; it relays to the long-lived knumble-tutor daemon over its socket.
package main

import (
	"os"

	acpbridge "github.com/haflettjm/llm-tutor/internal/app/acp"
)

const defaultSocket = "/tmp/llm-tutor.sock"

func main() {
	socket := os.Getenv("LLM_TUTOR_SOCKET")
	if socket == "" {
		socket = defaultSocket
	}
	acpbridge.Serve(acpbridge.NewHTTPClient(socket), os.Stdout, os.Stdin)
}
