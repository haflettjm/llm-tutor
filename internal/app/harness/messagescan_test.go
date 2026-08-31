package harness

import (
	"strings"
	"testing"
)

func TestMessageScannerExtractsAcrossFragmentBoundaries(t *testing.T) {
	var scanner messageScanner
	var got strings.Builder
	for _, fragment := range []string{`{"mess`, `age":"What ha`, `ppens if i st`, `arts at 1?","response_type":"question"}`} {
		got.WriteString(scanner.Feed(fragment))
	}
	if got.String() != "What happens if i starts at 1?" {
		t.Errorf("got %q", got.String())
	}
}

func TestMessageScannerDecodesEscapes(t *testing.T) {
	var scanner messageScanner
	got := scanner.Feed(`{"message":"line one\nline \"two\"","hint_level":0}`)
	want := "line one\nline \"two\""
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMessageScannerHandlesMessageNotFirst(t *testing.T) {
	var scanner messageScanner
	got := scanner.Feed(`{"response_type":"hint","message":"try tracing it"}`)
	if got != "try tracing it" {
		t.Errorf("got %q", got)
	}
}

func TestMessageScannerStopsAtEndOfValue(t *testing.T) {
	var scanner messageScanner
	first := scanner.Feed(`{"message":"done","concept_id":"PROG-004"}`)
	second := scanner.Feed(`trailing garbage`)
	if first != "done" || second != "" {
		t.Errorf("first=%q second=%q", first, second)
	}
}
