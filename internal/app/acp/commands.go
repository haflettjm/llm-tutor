package acpbridge

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/haflettjm/llm-tutor/internal/types/status"
)

// command is one entry in the editor's slash-command menu.
//
// Commands split by cost. A local command answers from daemon state and returns
// immediately. A routed command carries a directive into the model turn, because
// its result is teaching, not data -- "move to the next concept" should open
// that concept Socratically, not print a JSON blob at the learner.
type command struct {
	name        string
	description string
	inputHint   string // non-empty when the command takes an argument

	// local answers without a model turn. Exactly one of local/directive is set.
	local func(ctx context.Context, a *Agent, args string) (string, error)

	// directive turns the command into the learner message sent to the tutor.
	directive func(args string) string
}

// commands is populated in init rather than as a var literal: the help command
// renders the table it is itself a member of, which the compiler rejects as an
// initialization cycle when written as a plain literal.
var commands []command

func init() {
	commands = []command{
		{
			name:        "help",
			description: "What this tutor can do and how it works",
			local: func(context.Context, *Agent, string) (string, error) {
				return helpText(), nil
			},
		},
		{
			name:        "start",
			description: "Begin a session — the tutor orients itself and picks up where you left off",
			directive: func(args string) string {
				msg := "Starting a session. Call start_session first, then follow the session-start protocol " +
					"in your instructions for what it returns."
				if note := strings.TrimSpace(args); note != "" {
					msg += "\n\nWhat I want to work on: " + note
				}
				return msg
			},
			inputHint: "optional: what you want to work on",
		},
		{
			name:        "progress",
			description: "Where you are in the current lesson plan",
			local: func(ctx context.Context, a *Agent, _ string) (string, error) {
				p, err := a.client.Progress(ctx)
				if err != nil {
					return "", err
				}
				return renderProgress(p), nil
			},
		},
		{
			name:        "plans",
			description: "List the available lesson plan tracks",
			local: func(ctx context.Context, a *Agent, _ string) (string, error) {
				p, err := a.client.Plans(ctx)
				if err != nil {
					return "", err
				}
				return renderPlans(p), nil
			},
		},
		{
			name:        "switch",
			description: "Switch to a different lesson plan track",
			inputHint:   "track id, e.g. programming-fundamentals",
			local: func(ctx context.Context, a *Agent, args string) (string, error) {
				track := strings.TrimSpace(args)
				if track == "" {
					p, err := a.client.Plans(ctx)
					if err != nil {
						return "", err
					}
					return "Which track? Pass one of these ids to the switch command.\n\n" + renderPlans(p), nil
				}
				p, err := a.client.SetTrack(ctx, track)
				if err != nil {
					return "", err
				}
				return "Switched track.\n\n" + renderProgress(p), nil
			},
		},
		{
			name:        "next",
			description: "Move on to the next concept in the track",
			directive: func(string) string {
				return "I am ready to move on to the next concept. " +
					"Call get_next_concept to find out which one it is, then open it the way " +
					"MENTOR.md says a concept should be opened: with its diagnostic question, not a lecture."
			},
		},
		{
			name:        "end",
			description: "End this session and save what you learned about me",
			directive: func(args string) string {
				msg := "I am ending this session. Call end_session with a 2-3 sentence note on what I " +
					"worked on, what I actually demonstrated, and what to pick up next time. Also call " +
					"update_working_style with anything you noticed about how I learn. Then say goodbye briefly."
				if note := strings.TrimSpace(args); note != "" {
					msg += "\n\nMy own note on this session: " + note
				}
				return msg
			},
		},
	}
}

// availableCommands renders the command table for the editor's menu.
func availableCommands() []sdk.AvailableCommand {
	out := make([]sdk.AvailableCommand, 0, len(commands))
	for _, c := range commands {
		cmd := sdk.AvailableCommand{Name: c.name, Description: c.description}
		if c.inputHint != "" {
			cmd.Input = &sdk.AvailableCommandInput{
				Unstructured: &sdk.UnstructuredCommandInput{Hint: c.inputHint},
			}
		}
		out = append(out, cmd)
	}
	return out
}

// parseCommand recognises a leading slash command in the learner's text.
//
// A slash prefix alone is not enough: the first word must name a real command.
// Otherwise "/tmp/cache.go is throwing nil" -- a perfectly ordinary question --
// would be swallowed as an unknown command instead of being answered.
func parseCommand(text string) (cmd command, args string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return command{}, "", false
	}
	head, rest, _ := strings.Cut(strings.TrimPrefix(trimmed, "/"), " ")
	for _, c := range commands {
		if strings.EqualFold(head, c.name) {
			return c, strings.TrimSpace(rest), true
		}
	}
	return command{}, "", false
}

func helpText() string {
	var sb strings.Builder
	sb.WriteString("I am a Socratic programming tutor. I ask questions instead of handing you answers, ")
	sb.WriteString("and I track what you have actually demonstrated across sessions.\n\n")
	sb.WriteString("Just talk to me normally to work through something. Commands:\n\n")
	for _, c := range commands {
		sb.WriteString("  /")
		sb.WriteString(c.name)
		if c.inputHint != "" {
			sb.WriteString(" <")
			sb.WriteString(c.inputHint)
			sb.WriteString(">")
		}
		sb.WriteString("\n      ")
		sb.WriteString(c.description)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderProgress(p status.Progress) string {
	var sb strings.Builder

	if p.Track == "" {
		sb.WriteString("No lesson plan selected yet.\n")
		if p.Note != "" {
			sb.WriteString("\n" + p.Note + "\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	title := p.TrackTitle
	if title == "" {
		title = p.Track
	}
	fmt.Fprintf(&sb, "%s\n", title)
	if p.Total > 0 {
		fmt.Fprintf(&sb, "  %d/%d concepts demonstrated%s\n", p.Demonstrated, p.Total, progressBar(p.Demonstrated, p.Total))
	}
	if p.Learning > 0 {
		fmt.Fprintf(&sb, "  %d in progress\n", p.Learning)
	}
	if p.Review > 0 {
		fmt.Fprintf(&sb, "  %d due for review\n", p.Review)
	}
	fmt.Fprintf(&sb, "  %d sessions so far\n", p.Sessions)
	if p.ActiveSoul != "" {
		fmt.Fprintf(&sb, "  teaching as: %s\n", p.ActiveSoul)
	}

	if p.NextConcept != nil {
		c := p.NextConcept
		fmt.Fprintf(&sb, "\nUp next -- %s: %s", c.ID, c.Title)
		if p.Position > 0 {
			fmt.Fprintf(&sb, " (%d of %d)", p.Position, p.Total)
		}
		sb.WriteString("\n")
		if c.Objective != "" {
			fmt.Fprintf(&sb, "  Goal: %s\n", c.Objective)
		}
		if c.Evidence != "" {
			fmt.Fprintf(&sb, "  Done when: %s\n", c.Evidence)
		}
	}
	if p.Note != "" {
		sb.WriteString("\n" + p.Note + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// progressBar is a compact visual for a ratio, omitted when there is nothing
// meaningful to show.
func progressBar(done, total int) string {
	if total <= 0 {
		return ""
	}
	const width = 20
	filled := done * width / total
	if filled > width {
		filled = width
	}
	return "  [" + strings.Repeat("#", filled) + strings.Repeat(".", width-filled) + "]"
}

func renderPlans(p status.Plans) string {
	if len(p.Plans) == 0 {
		return "No lesson plans found. Check the lesson-plans directory in your data dir."
	}
	var sb strings.Builder
	sb.WriteString("Lesson plan tracks:\n\n")
	for _, s := range p.Plans {
		marker := "  "
		if s.ID == p.Active {
			marker = "* "
		}
		fmt.Fprintf(&sb, "%s%s\n", marker, s.ID)
		fmt.Fprintf(&sb, "      %s", s.Title)
		if s.Concepts > 0 {
			fmt.Fprintf(&sb, " -- %d concepts", s.Concepts)
		}
		sb.WriteString("\n")
	}
	if p.Active != "" {
		sb.WriteString("\n* = active track")
	}
	return strings.TrimRight(sb.String(), "\n")
}
