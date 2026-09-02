package harness

import (
	"strings"
	"testing"
)

func TestMessageScannerAcceptsWhitespaceAfterColon(t *testing.T) {
	var scanner messageScanner
	if got := scanner.Feed(`{"message": "Hello"}`); got != "Hello" {
		t.Fatalf("text = %q", got)
	}
}

func TestMessageScannerExtractsAcrossFragmentBoundaries(t *testing.T) {
	var scanner messageScanner
	var got strings.Builder
	for _, fragment := range []string{`{"mess`, `age":"What ha`, `ppens if i st`, `arts at 1?","response_type":"question"}`} {
		got.WriteString(scanner.Feed(fragment))
	}
	if got.String() != "What happens if i starts at 1?" {
		t.Fatalf("text = %q", got.String())
	}
}

func TestMessageScannerDecodesEscapes(t *testing.T) {
	var scanner messageScanner
	if got := scanner.Feed(`{"message":"Say \"hi\"\\now"}`); got != "Say \"hi\"\\now" {
		t.Fatalf("text = %q", got)
	}
}

func TestMessageScannerHandlesMessageNotFirst(t *testing.T) {
	var scanner messageScanner
	if got := scanner.Feed(`{"hint_level":0,"message":"hi"}`); got != "hi" {
		t.Fatalf("text = %q", got)
	}
}

func TestMessageScannerStopsAtEndOfValue(t *testing.T) {
	var scanner messageScanner
	if got := scanner.Feed(`{"message":"hi","response_type":"question"}`); got != "hi" {
		t.Fatalf("text = %q", got)
	}
}
