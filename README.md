# llm-tutor

A Socratic programming tutor that lives in your editor. It asks questions
instead of handing you answers, and it remembers what you have actually
demonstrated across sessions.

It teaches any language. Examples and exercises come back in whatever language
your current diff is written in.

```text
Editor                          knumble-tutor (daemon)
  ├── Neovim plugin ──┐           ├── MENTOR.md      universal teaching contract
  │   (unix socket)   ├──────────▶├── souls/         role-specific personas
  └── Zed / ACP ──────┘           ├── lesson-plans/  concept tracks
      (knumble-acp,               ├── learner state  progress · profile · scratchpad
       stdio JSON-RPC)            │
                                  ├── drives the `claude` CLI per turn
                                  └── hosts an MCP server the CLI calls back into
```

The daemon does not call a model API directly. It composes a system prompt from
`MENTOR.md` plus the soul for your current concept, writes it where the harness
expects it, and drives the harness CLI. The harness reaches back into the daemon
over MCP to read and update your learning record — so the tutor's memory and the
tutor's reasoning stay in separate processes.

## Requirements

- Go 1.26+
- The [`claude` CLI](https://claude.com/claude-code), authenticated
  (`claude` is the only harness with a working `Query` today)

No `ANTHROPIC_API_KEY` is needed — the daemon shells out to the harness CLI,
which uses its own credentials.

## Install

```bash
make build          # builds bin/knumble-tutor and bin/knumble-acp
make install        # copies both into ~/.local/bin
```

## First run

```bash
./bin/knumble-tutor
```

The first start creates `~/Documents/llm-tutor/` (override with
`LLM_TUTOR_DATA`), seeds the content files, and writes a `config.json`
template. Set `harness` in it, then start again:

```json
{
  "harness": "claude-code",
  "socket": "/tmp/llm-tutor.sock",
  "mcp_addr": ":7890"
}
```

Starting the daemon registers its MCP server in `~/.claude.json`. Existing
entries and settings are preserved.

That is all the setup there is. **You do not need to pick a lesson plan.**
Open your editor and run the `start` command; the tutor works out the rest.

## Editors

### Neovim

```lua
{ dir = "~/git/personal/llm-tutor/plugin/nvim", name = "llm-tutor",
  config = function() require("llm-tutor").setup() end }
```

| Command | |
|---|---|
| `:LlmStart` | begin a session — the tutor orients itself |
| `:LlmAsk` | ask a question |
| `:LlmAskDiff` | ask with the current `git diff HEAD` attached |
| `:LlmProgress` | where you are in the track |
| `:LlmPlans` | list tracks |
| `:LlmTrack [id]` | switch track (prompts if no id) |
| `:LlmNext` | move on to the next concept |
| `:LlmEnd [note]` | end the session and save what was learned |
| `:LlmHealth` | check the daemon is up |

Bound by default: `<leader>ts` start · `<leader>ta` ask · `<leader>td` ask with
diff · `<leader>tp` progress · `<leader>tn` next.

### Zed, and any ACP client

`knumble-acp` speaks the Agent Client Protocol over stdio and relays to the
daemon. Register it in Zed's `settings.json` (schema as of Zed 1.14):

```json
{
  "agent_servers": {
    "knumble-tutor": {
      "type": "custom",
      "command": "/absolute/path/to/knumble-acp",
      "args": [],
      "env": {}
    }
  }
}
```

`type` and an absolute `command` are both required — Zed 1.14 does not accept
the older bare `{ "command": ... }` form, and does not expand `~`.

The agent then appears in the agent panel's new-thread menu.

**Verification status:** the ACP round-trip is verified against a scratch
JSON-RPC client. It has not been driven from Zed's agent panel or a Neovim ACP
client, so the rendering of the command menu and transcript in those clients is
unconfirmed.

It publishes `/start`, `/help`, `/progress`, `/plans`, `/switch`, `/next` and
`/end` to the client's command menu. `/progress`, `/plans` and `/switch` are
answered from local state and cost no model turn.

Point it at a non-default socket with `LLM_TUTOR_SOCKET`.

## How a session goes

Run `/start` (or `:LlmStart`). The tutor calls `start_session`, which tells it
whether you are new, what it already knows, what it does *not* know, and what
happened last time.

**First time**, it will not open a lesson. It asks what you want to be able to
do and what you are working on — two or three questions, then it starts
teaching against your own code.

**After that**, it opens by reconnecting to something specific from last time,
then asks what you want to work on now.

```
/start                    begin, and pick up the thread
<paste code, ask things>  the normal case
/end                      writes down what it learned about you
```

Say **"just tell me"** at any point and it gives you the full answer
immediately, no guilt-tripping. That is a designed escape hatch.

## It is not limited to the lesson plans

Bring it anything: a failing Terraform plan, a slow SQL query, a language it has
no track for. That is the normal case, not a fallback.

When you do, the tutor calls `set_focus` to remember what you are working on,
invents a stable concept id (`SQL-SEQ-SCAN`, `TF-STATE-DRIFT`) so your progress
still accumulates, and calls `set_soul` to become the right kind of teacher:

| You have | It becomes |
|---|---|
| A bug | `debugging-coach` |
| Working code that should be better | `code-review` |
| A concept you have no model for | `concepts-tutor` |

Lesson plans are optional scaffolding for when you want a structured path
instead. `/progress` counts your off-plan work separately.

## Daemon API

Served over the unix socket.

| | | |
|---|---|---|
| `POST` | `/tutor` | one tutoring turn (costs a model call) |
| `GET` | `/progress` | position in the track, next concept |
| `GET` | `/plans` | available tracks |
| `POST` | `/track` | switch track |
| `GET` | `/health` | harness, active soul, plans loaded |

## Data directory

```text
~/Documents/llm-tutor/
├── config.json              harness selection, socket, MCP address
├── MENTOR.md                the teaching contract — edit this
├── souls/                   personas, selected per concept
├── lesson-plans/            concept tracks
├── progress.json            per-concept state
├── learner-profile.json     who you are, how you learn
├── scratchpad.json          the tutor's working memory this session
├── learning-events.jsonl    append-only turn log
└── CLAUDE.md                generated each turn — do not edit
```

`MENTOR.md`, the souls, and the lesson plans are re-read between turns, so you
can edit them while the daemon runs.

They are also seeded from the binary. A file you have edited is never
overwritten; one you have not touched follows the shipped version, tracked by
checksum in `.seeded.json`. The daemon logs which files it kept, updated, or
created at startup.

## Lesson plans

`programming-fundamentals` is the only track with concept-level entries so far —
10 concepts, entry level, language-agnostic. The other five
(`algorithms-and-data-structures`, `architecture`, `backend-engineering`,
`devops-and-infrastructure`, `systems-and-concurrency`) carry goals and soul
mappings but their concepts are still being written.

A concept is parsed from a `### ID: Title` block:

```markdown
### PROG-004: Loops and Iteration
- **Objective:** Traces the loop variable through each iteration.
- **Diagnostic:** "How many times does this loop run?"
- **Exercise:** Write a loop summing 1 through 10.
- **Misconception:** The loop variable equals its last in-loop value after exit.
- **Evidence:** Fills a 5-row trace table correctly.
- **Transfer:** Identify an off-by-one without running the code.
- **Prerequisites:** PROG-003
- **Soul:** debugging-coach
```

`Evidence` is what the tutor must observe before marking a concept demonstrated.
`Soul` is optional and overrides the plan's soul-mapping table.

## Status

Working end to end on the `claude-code` harness: tutoring turns, conversation
resume, per-concept soul selection, progress tracking, both editor paths.

Not done: `opencode`, `codex` and `hermes` harnesses check for their binary but
their `Query` is unimplemented. Concept-level entries for five of the six tracks.
Streaming — replies arrive whole, after ~15-40s.

## Development

```bash
make test     # go test ./...
make check    # fmt, vet, test
make run      # build and run the daemon
```
