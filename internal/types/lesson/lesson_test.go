package lesson

import (
	"path/filepath"
	"testing"
)

// contentDir is the shipped lesson plan content. Parsing is tested against the
// real files rather than fixtures so a format drift in content fails the build.
const contentDir = "../../app/content/lesson-plans"

func TestLoadParsesShippedProgrammingFundamentals(t *testing.T) {
	p, err := Load(filepath.Join(contentDir, "programming-fundamentals.md"))
	if err != nil {
		t.Fatal(err)
	}

	if p.ID != "programming-fundamentals" {
		t.Errorf("id = %q", p.ID)
	}
	if p.Title != "Programming Fundamentals" {
		t.Errorf("title = %q", p.Title)
	}
	if p.Language != "agnostic" {
		t.Errorf("language = %q", p.Language)
	}
	if len(p.Concepts) != 10 {
		t.Fatalf("parsed %d concepts, want 10", len(p.Concepts))
	}
	if p.Goal == "" {
		t.Error("learning goal was not captured")
	}

	c, ok := p.Concept("PROG-004")
	if !ok {
		t.Fatal("PROG-004 missing")
	}
	if c.Title != "Loops and Iteration" {
		t.Errorf("title = %q", c.Title)
	}
	if c.Objective == "" || c.Diagnostic == "" || c.Evidence == "" {
		t.Errorf("fields not populated: %+v", c)
	}
	if len(c.Prerequisites) != 1 || c.Prerequisites[0] != "PROG-003" {
		t.Errorf("prerequisites = %v", c.Prerequisites)
	}
}

func TestPrerequisitesNoneParsesEmpty(t *testing.T) {
	p, err := Load(filepath.Join(contentDir, "programming-fundamentals.md"))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := p.Concept("PROG-001")
	if len(c.Prerequisites) != 0 {
		t.Errorf("PROG-001 prerequisites = %v, want none", c.Prerequisites)
	}
}

func TestMultiplePrerequisitesParse(t *testing.T) {
	p, err := Load(filepath.Join(contentDir, "programming-fundamentals.md"))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := p.Concept("PROG-009")
	if len(c.Prerequisites) != 2 {
		t.Fatalf("PROG-009 prerequisites = %v, want 2", c.Prerequisites)
	}
}

func TestOrderFollowsDeclaredSequence(t *testing.T) {
	p, err := Load(filepath.Join(contentDir, "programming-fundamentals.md"))
	if err != nil {
		t.Fatal(err)
	}
	order := p.Order()
	if len(order) != 10 {
		t.Fatalf("order has %d entries, want 10", len(order))
	}
	if order[0] != "PROG-001" || order[9] != "PROG-010" {
		t.Errorf("order = %v", order)
	}
}

func TestSoulForUsesRangeTableThenPerConceptOverride(t *testing.T) {
	p, err := Load(filepath.Join(contentDir, "programming-fundamentals.md"))
	if err != nil {
		t.Fatal(err)
	}
	// "| PROG-001 through PROG-008 | concepts-tutor |"
	if got := p.SoulFor("PROG-005"); got != "concepts-tutor" {
		t.Errorf("SoulFor(PROG-005) = %q, want concepts-tutor", got)
	}
	// PROG-009 carries "- **Soul:** debugging-coach" AND a table row; both agree.
	if got := p.SoulFor("PROG-009"); got != "debugging-coach" {
		t.Errorf("SoulFor(PROG-009) = %q, want debugging-coach", got)
	}
	if got := p.SoulFor("PROG-010"); got != "code-review" {
		t.Errorf("SoulFor(PROG-010) = %q, want code-review", got)
	}
	if got := p.SoulFor("NOPE-001"); got != "" {
		t.Errorf("unknown concept resolved to %q, want empty", got)
	}
}

func TestPerConceptSoulOverridesTable(t *testing.T) {
	p := &Plan{
		Concepts:  []Concept{{ID: "X-001", Soul: "debugging-coach"}},
		soulRules: []soulRule{{prefix: "X", low: 1, high: 9, soul: "concepts-tutor"}},
	}
	if got := p.SoulFor("X-001"); got != "debugging-coach" {
		t.Errorf("per-concept Soul did not win: got %q", got)
	}
}

func TestSoulTableSkipsProseKeyedRowsAndHeaders(t *testing.T) {
	if _, ok := parseSoulRow("| Concepts and mental models | concepts-tutor |"); ok {
		t.Error("prose-keyed row should not produce a rule")
	}
	if _, ok := parseSoulRow("| Concept | Soul |"); ok {
		t.Error("header row should not produce a rule")
	}
	if _, ok := parseSoulRow("|---|---|"); ok {
		t.Error("separator row should not produce a rule")
	}
}

func TestSoulTableStripsDeferredMarker(t *testing.T) {
	rule, ok := parseSoulRow("| ALG-001 through ALG-004 | algorithms-tutor (deferred) |")
	if !ok {
		t.Fatal("row did not parse")
	}
	if rule.soul != "algorithms-tutor" {
		t.Errorf("soul = %q, want algorithms-tutor", rule.soul)
	}
}

func TestLoadDirParsesEveryShippedPlan(t *testing.T) {
	plans, err := LoadDir(contentDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 6 {
		t.Fatalf("loaded %d plans, want 6: %v", len(plans), IDs(plans))
	}
	for id, p := range plans {
		if p.Title == "" {
			t.Errorf("plan %s has no title", id)
		}
	}
}

func TestOrderAppendsConceptsMissingFromSequence(t *testing.T) {
	p := &Plan{
		Sequence: []string{"A-001", "A-999"}, // A-999 does not exist
		Concepts: []Concept{{ID: "A-002"}, {ID: "A-001"}},
	}
	order := p.Order()
	if len(order) != 2 || order[0] != "A-001" || order[1] != "A-002" {
		t.Fatalf("order = %v, want [A-001 A-002]", order)
	}
}
