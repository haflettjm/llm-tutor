package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haflettjm/llm-tutor/internal/types/request"
)

func TestStreamQueryEmitsIncrementalTextAndReturnsFullResponse(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "claude")
	output := `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"message\":\"What "}}}` + "\n" +
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"changes?\"}"}}}` + "\n" +
		`{"type":"result","subtype":"success","result":"{\"message\":\"What changes?\",\"response_type\":\"question\",\"hint_level\":0}"}` + "\n"
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s' '"+output+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := newClaudeCode(dir)
	var got strings.Builder
	resp, err := c.StreamQuery(context.Background(), request.Request{Message: "hi", SessionID: "s1"}, func(ch StreamChunk) error {
		got.WriteString(ch.Text)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamQuery: %v", err)
	}
	if got.String() != "What changes?" || resp.Message != "What changes?" {
		t.Fatalf("streamed=%q response=%+v", got.String(), resp)
	}
}
