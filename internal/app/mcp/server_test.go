package mcp

import (
	"strings"
	"testing"
)

func TestParseProbesKeepsKnownValues(t *testing.T) {
	got := parseProbes("prediction, teach-back,trace-table")
	want := []string{"prediction", "teach-back", "trace-table"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestParseProbesNormalisesCaseAndSpacing(t *testing.T) {
	got := parseProbes("  PREDICTION ,  Teach-Back ")
	if len(got) != 2 || got[0] != "prediction" || got[1] != "teach-back" {
		t.Fatalf("got %v", got)
	}
}

// The bug this exists to prevent: a sentence written into a "comma-separated"
// field becomes several fragments that persist as if they were probe names.
func TestParseProbesRejectsProse(t *testing.T) {
	prose := "Not yet determinable - one question asked, none answered. " +
		"Do not read the unanswered diagnostic as prediction-probes failing."
	if got := parseProbes(prose); len(got) != 0 {
		t.Fatalf("prose leaked into the profile as %v", got)
	}
}

func TestParseProbesDropsUnknownValuesButKeepsKnownOnes(t *testing.T) {
	got := parseProbes("prediction, vibes, teach-back, guessing")
	if len(got) != 2 || got[0] != "prediction" || got[1] != "teach-back" {
		t.Fatalf("got %v, want [prediction teach-back]", got)
	}
}

func TestParseProbesHandlesEmptyInput(t *testing.T) {
	if got := parseProbes(""); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

// The tool description must advertise the same vocabulary the parser enforces,
// or the model is being asked to guess.
func TestProbeVocabularyMatchesTheFilter(t *testing.T) {
	for _, v := range strings.Split(probeVocabulary, ", ") {
		if !probeTypes[v] {
			t.Errorf("%q is advertised but would be filtered out", v)
		}
	}
	if n := len(strings.Split(probeVocabulary, ", ")); n != len(probeTypes) {
		t.Errorf("vocabulary lists %d values, filter accepts %d", n, len(probeTypes))
	}
}

// Tech stacks are open-ended and must not be filtered.
func TestSplitCommaKeepsArbitraryValues(t *testing.T) {
	got := splitComma("go, postgres , gin,  ")
	if len(got) != 3 || got[0] != "go" || got[2] != "gin" {
		t.Fatalf("got %v", got)
	}
}

func TestIndexOf(t *testing.T) {
	ids := []string{"A", "B", "C"}
	if indexOf(ids, "B") != 1 {
		t.Error("indexOf did not find B")
	}
	if indexOf(ids, "Z") != -1 {
		t.Error("indexOf should return -1 for a missing id")
	}
}
