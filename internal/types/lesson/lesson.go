// Package lesson parses lesson plan markdown files into structured concepts.
//
// A plan file has three machine-read sections: YAML frontmatter, a "Soul
// mapping" table, and a "## Concepts" section of "### ID: Title" blocks with
// bolded field lines. Everything else in the file is prose for the human and is
// deliberately ignored.
package lesson

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Concept is one teachable unit from a lesson plan.
type Concept struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Objective     string   `json:"objective,omitempty"`
	Diagnostic    string   `json:"diagnostic,omitempty"`
	Exercise      string   `json:"exercise,omitempty"`
	Misconception string   `json:"misconception,omitempty"`
	Evidence      string   `json:"evidence,omitempty"`
	Transfer      string   `json:"transfer,omitempty"`
	Prerequisites []string `json:"prerequisites,omitempty"`

	// Soul is an optional per-concept override written as "- **Soul:** name".
	// It wins over the plan-level soul mapping table.
	Soul string `json:"soul,omitempty"`
}

// soulRule maps a contiguous range of concept IDs to a soul. It is built from
// rows of the "Soul mapping" table such as "PROG-001 through PROG-008".
type soulRule struct {
	prefix    string
	low, high int
	soul      string
}

// Plan is a parsed lesson plan.
type Plan struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Language string    `json:"language,omitempty"`
	Version  int       `json:"version,omitempty"`
	Goal     string    `json:"goal,omitempty"`
	Sequence []string  `json:"sequence,omitempty"`
	Concepts []Concept `json:"concepts"`

	soulRules []soulRule
	byID      map[string]int
}

var (
	frontmatterKV = regexp.MustCompile(`^([a-z_]+):\s*(.*)$`)
	conceptHead   = regexp.MustCompile(`^###\s+([A-Za-z]+-\d+)\s*:\s*(.+)$`)
	fieldLine     = regexp.MustCompile(`^-\s+\*\*([A-Za-z]+):\*\*\s*(.+)$`)
	conceptID     = regexp.MustCompile(`^([A-Za-z]+)-(\d+)$`)
	tableRow      = regexp.MustCompile(`^\|(.+)\|(.+)\|$`)
)

// Load reads and parses a single lesson plan file.
func Load(path string) (*Plan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open lesson plan %s: %w", path, err)
	}
	defer f.Close()

	p, err := parse(bufio.NewScanner(f))
	if err != nil {
		return nil, fmt.Errorf("parse lesson plan %s: %w", path, err)
	}
	if p.ID == "" {
		p.ID = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	p.index()
	return p, nil
}

// LoadDir parses every *.md file in dir, keyed by plan ID. A file that fails to
// parse is skipped rather than failing the whole directory: one malformed plan
// must not take the tutor down.
func LoadDir(dir string) (map[string]*Plan, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	plans := make(map[string]*Plan, len(matches))
	for _, m := range matches {
		p, err := Load(m)
		if err != nil {
			continue
		}
		plans[p.ID] = p
	}
	return plans, nil
}

// section tracks which "## " block the scanner is inside, because the same
// line shapes (table rows, arrows) appear in more than one section.
type section int

const (
	sectionOther section = iota
	sectionFrontmatter
	sectionGoal
	sectionSoulMapping
	sectionSequence
	sectionConcepts
)

func parse(sc *bufio.Scanner) (*Plan, error) {
	p := &Plan{}
	var (
		sec       section
		goal      strings.Builder
		current   *Concept
		lineCount int
	)

	flush := func() {
		if current != nil {
			p.Concepts = append(p.Concepts, *current)
			current = nil
		}
	}

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		lineCount++

		// Frontmatter is delimited by --- on the very first line.
		if trimmed == "---" {
			if lineCount == 1 {
				sec = sectionFrontmatter
				continue
			}
			if sec == sectionFrontmatter {
				sec = sectionOther
				continue
			}
		}

		if strings.HasPrefix(trimmed, "## ") {
			flush()
			switch strings.ToLower(strings.TrimSpace(trimmed[3:])) {
			case "learning goal":
				sec = sectionGoal
			case "soul mapping":
				sec = sectionSoulMapping
			case "prerequisite sequence":
				sec = sectionSequence
			default:
				// "## Concepts" and "## Concept areas (…)" both hold concepts.
				if strings.HasPrefix(strings.ToLower(trimmed), "## concept") {
					sec = sectionConcepts
				} else {
					sec = sectionOther
				}
			}
			continue
		}

		switch sec {
		case sectionFrontmatter:
			if m := frontmatterKV.FindStringSubmatch(trimmed); m != nil {
				p.setMeta(m[1], strings.Trim(m[2], `"`))
			}

		case sectionGoal:
			if trimmed != "" {
				if goal.Len() > 0 {
					goal.WriteByte(' ')
				}
				goal.WriteString(trimmed)
			}

		case sectionSoulMapping:
			if rule, ok := parseSoulRow(trimmed); ok {
				p.soulRules = append(p.soulRules, rule)
			}

		case sectionSequence:
			if strings.Contains(trimmed, "->") {
				for _, id := range strings.Split(trimmed, "->") {
					if id := strings.TrimSpace(id); conceptID.MatchString(id) {
						p.Sequence = append(p.Sequence, id)
					}
				}
			}

		case sectionConcepts:
			if m := conceptHead.FindStringSubmatch(trimmed); m != nil {
				flush()
				current = &Concept{ID: m[1], Title: strings.TrimSpace(m[2])}
				continue
			}
			if current == nil {
				continue
			}
			if m := fieldLine.FindStringSubmatch(trimmed); m != nil {
				current.set(m[1], strings.TrimSpace(m[2]))
			}
		}
	}
	flush()

	if err := sc.Err(); err != nil {
		return nil, err
	}
	p.Goal = goal.String()
	return p, nil
}

func (p *Plan) setMeta(key, value string) {
	switch key {
	case "id":
		p.ID = value
	case "title":
		p.Title = value
	case "language":
		p.Language = value
	case "version":
		p.Version, _ = strconv.Atoi(value)
	}
}

func (c *Concept) set(field, value string) {
	switch strings.ToLower(field) {
	case "objective":
		c.Objective = value
	case "diagnostic":
		c.Diagnostic = value
	case "exercise":
		c.Exercise = value
	case "misconception":
		c.Misconception = value
	case "evidence":
		c.Evidence = value
	case "transfer":
		c.Transfer = value
	case "soul":
		c.Soul = value
	case "prerequisites":
		if strings.EqualFold(value, "none") {
			return
		}
		for _, id := range strings.Split(value, ",") {
			if id := strings.TrimSpace(id); conceptID.MatchString(id) {
				c.Prerequisites = append(c.Prerequisites, id)
			}
		}
	}
}

// parseSoulRow reads one row of the soul mapping table. Rows keyed by prose
// ("Concepts and mental models") carry no concept IDs and are skipped -- only
// ID-keyed rows can be resolved mechanically.
func parseSoulRow(line string) (soulRule, bool) {
	m := tableRow.FindStringSubmatch(line)
	if m == nil {
		return soulRule{}, false
	}
	key := strings.TrimSpace(m[1])
	soul := strings.TrimSpace(m[2])

	// "code-review (deferred)" -- a soul that is documented but not written yet.
	if i := strings.Index(soul, "("); i >= 0 {
		soul = strings.TrimSpace(soul[:i])
	}
	if soul == "" || strings.EqualFold(soul, "soul") {
		return soulRule{}, false
	}

	lowID, highID := key, key
	if parts := strings.Split(key, " through "); len(parts) == 2 {
		lowID, highID = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}

	lowM, highM := conceptID.FindStringSubmatch(lowID), conceptID.FindStringSubmatch(highID)
	if lowM == nil || highM == nil || lowM[1] != highM[1] {
		return soulRule{}, false
	}
	low, _ := strconv.Atoi(lowM[2])
	high, _ := strconv.Atoi(highM[2])
	if low > high {
		low, high = high, low
	}
	return soulRule{prefix: lowM[1], low: low, high: high, soul: soul}, true
}

func (p *Plan) index() {
	p.byID = make(map[string]int, len(p.Concepts))
	for i, c := range p.Concepts {
		p.byID[c.ID] = i
	}
}

// Concept returns the concept with the given ID.
func (p *Plan) Concept(id string) (Concept, bool) {
	if p.byID == nil {
		p.index()
	}
	i, ok := p.byID[id]
	if !ok {
		return Concept{}, false
	}
	return p.Concepts[i], true
}

// Order returns concept IDs in teaching order: the declared prerequisite
// sequence when present, otherwise the order they appear in the file. IDs named
// in the sequence but missing from the concept list are dropped, and concepts
// missing from the sequence are appended so nothing becomes unreachable.
func (p *Plan) Order() []string {
	if p.byID == nil {
		p.index()
	}
	seen := make(map[string]bool, len(p.Concepts))
	order := make([]string, 0, len(p.Concepts))
	for _, id := range p.Sequence {
		if _, ok := p.byID[id]; ok && !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}
	for _, c := range p.Concepts {
		if !seen[c.ID] {
			seen[c.ID] = true
			order = append(order, c.ID)
		}
	}
	return order
}

// SoulFor resolves which soul should teach a concept: an explicit per-concept
// "**Soul:**" line wins, then the soul mapping table. Returns "" when the plan
// says nothing, leaving the choice to the caller's default.
func (p *Plan) SoulFor(id string) string {
	if c, ok := p.Concept(id); ok && c.Soul != "" {
		return c.Soul
	}
	m := conceptID.FindStringSubmatch(id)
	if m == nil {
		return ""
	}
	n, _ := strconv.Atoi(m[2])
	for _, r := range p.soulRules {
		if r.prefix == m[1] && n >= r.low && n <= r.high {
			return r.soul
		}
	}
	return ""
}

// IDs returns every plan ID in a map, sorted, for stable listings.
func IDs(plans map[string]*Plan) []string {
	ids := make([]string, 0, len(plans))
	for id := range plans {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
