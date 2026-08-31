package harness

import "testing"

func TestCanStreamReportsFalseForNonStreamingHarness(t *testing.T) {
	h := &codex{Base: Base{promptFile: "CODEX.md"}}
	if _, ok := CanStream(h); ok {
		t.Fatal("codex must not report streaming support")
	}
}

func TestCanStreamReportsTrueForClaudeCode(t *testing.T) {
	h := newClaudeCode(t.TempDir())
	if _, ok := CanStream(h); !ok {
		t.Fatal("claude-code must report streaming support")
	}
}

func TestStreamChunkCarriesTextAndReset(t *testing.T) {
	chunk := StreamChunk{Text: "try tracing i", Reset: true}
	if chunk.Text != "try tracing i" || !chunk.Reset {
		t.Fatalf("chunk = %#v", chunk)
	}
}
