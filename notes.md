# Notes

Scratch notes and decision records. Not polished docs — move things into
`docs/` once they settle.

---

## 2026-08-21 (later) — The syllabus became scaffolding, not a gate

The tutor only worked if you first picked a lesson plan, and only one plan had
concepts. That made it a syllabus reader, not a tutor. Real sessions start from
the learner's own work.

**What changed**

- `start_session` (new MCP tool) returns a briefing: first-session or
  returning, what is known, **what is not known**, last sessions, carried-over
  scratchpad, track position. `MENTOR.md` now has an explicit session-start
  protocol keyed off it, different for a new vs returning learner.
- `set_focus` records what is being worked on when it is not a plan concept.
  `update_concept_state` takes invented ids (`SQL-SEQ-SCAN`), so off-plan
  learning still accumulates. `/progress` counts it separately.
- `set_soul` lets the tutor pick its own persona from the situation rather than
  from track position, and it beats the plan mapping. A learner four concepts
  from the debugging track who hits a nil deref needs the debugging coach now.
- `/start` command in both editors.

**Two things the live runs taught**

- *The soul override could not affect the turn that set it.* The system prompt
  is composed before the harness runs. Fixed by re-composing at the **end** of
  a turn too, so a persona chosen mid-turn is live before the next one and the
  daemon reports it honestly. Cheap: the write is skipped unless content
  changed.
- *`exit status 1: ` with nothing after it.* The CLI reports API-level failures
  as a JSON envelope on **stdout** and exits non-zero with an empty stderr. We
  were throwing stdout away on error, producing an error message containing no
  information. `claudeError` now prefers the stdout envelope, falls back to
  stderr, and says "wrote nothing" when both are empty. Resume detection reads
  both streams for the same reason.

**Verified live:** a first session that asks who you are instead of opening a
lesson; an off-syllabus Postgres question taught Socratically with `set_focus`,
an invented concept id and an automatic switch to `debugging-coach`; and a
**brand-new session recalling the previous one** by its specifics, including
noticing it had asked for the same evidence twice and never received it.

---

## 2026-08-21 — Implemented: stages 0-1, plus the tutoring loop closes

Stages 0 and 1 of the migration below are done and verified end to end against
`claude` 2.1.234.

Stages 2 and 3 are **code-complete but unverified in the real editors.** The
protocol round-trip was driven with a scratch JSON-RPC client, not with
codecompanion.nvim or Zed's agent panel, so their gates are not met. The Zed
`agent_servers` config in the README is written from the docs, not from a
session. Stage 4 (user service, auto-start) is untouched.

**Working:**

- Lesson plans parse (`internal/types/lesson`). Concepts, prerequisite
  sequence, and soul mapping all come out of the markdown.
- Soul selection follows the learner. The system prompt is recomposed before
  every turn and rewritten only when the composed content actually changes, so
  `MENTOR.md` and the souls can be edited while the daemon runs.
- The daemon grew a state API (`/progress`, `/plans`, `/track`, `/health`)
  alongside `/tutor`. Commands that only read state never spend a model turn.
- `knumble-acp` publishes six commands to the client menu and cancels
  in-flight turns properly. Verified by driving raw JSON-RPC over stdio --
  not yet by an actual editor client.
- The Neovim plugin has parity: `:LlmProgress`, `:LlmPlans`, `:LlmTrack`,
  `:LlmNext`, `:LlmEnd`, `:LlmHealth`.
- Content seeding is checksum-tracked (`.seeded.json`). Untouched files follow
  the shipped version; edited files are kept and reported at startup.

**Verified in a real session:** two-turn conversation with `--resume` holding
context, `get_next_concept` opening PROG-001 with its diagnostic, and
`end_session` writing a session note that corrected a false claim the learner
made about what happened. `MENTOR.md` is doing real work.

**Findings worth keeping:**

- A pruned session and a bad CLI flag both exit `1`. Only stderr
  distinguishes them, which is why `decideResumeRecovery` takes both. Claude
  2.1.234 prints `No conversation found with session ID: <uuid>`; there is a
  regression test pinned to that string.
- Fields described to a model as "comma-separated" will sometimes receive a
  sentence. `ineffective_probes` got prose that `strings.Split` shredded into
  two fake probe names. Closed vocabularies now filter it.
- Five of six lesson plans have no concept-level entries, so they parse to
  zero concepts. "No concepts yet" and "all concepts demonstrated" had to be
  reported differently -- conflating them tells a learner they finished a track
  that was never written.

**Still open:**

- `opencode`, `codex`, `hermes`: `Start` checks for the binary, `Query` returns
  "not implemented".
- Concept-level entries for the five deferred tracks.
- No streaming. Replies land whole after 15-50s.
- Stage 4: systemd user service, adapter auto-start/reconnect.
- Stage 2/3 gates: run this against codecompanion.nvim and Zed's agent panel
  and confirm the command menu and transcript actually render.

---

## 2026-08-17 — Editor integration: adopt ACP, keep the daemon

**Status:** decided and implemented -- see the entry above.
Full research writeup to be added to `docs/` later.

### The question

The daemon has almost no human-facing command surface (`POST /tutor`,
`GET /health`). We want `help`, `progress`, `next concept`, `end session`,
`switch lesson plan` — working in both Neovim and Zed, without building it
twice.

### Recommendation

**Adopt ACP (Agent Client Protocol) as the editor-facing transport.** Keep
the long-lived daemon. Keep MCP inward-facing. Retire the bespoke
curl-over-unix-socket path and the Neovim floating window.

```
Editor (Zed Agent Panel | Neovim ACP client)
  │ spawns per-workspace; JSON-RPC 2.0 over stdio (ACP)
  ▼
knumble-acp  (NEW, thin — the only new component)
  │ relays over /tmp/llm-tutor.sock
  ▼
knumble-tutor  (daemon, core unchanged)
  ├── composes MENTOR.md + soul → CLAUDE.md/AGENTS.md/CODEX.md
  ├── owns learner state (progress, profile, scratchpad, events)
  ├── spawns + supervises child AI CLI
  └── MCP server (SSE/TCP) ← child AI CLI calls back in
```

### Why, in three points

1. **LSP is the wrong contract for the conversation.** `executeCommand` +
   `executeCommandProvider` can advertise a command *list*, but LSP has no
   multi-turn chat primitive — `showMessage`/`showMessageRequest`/
   `showDocument` are notification affordances, not a transcript. Copilot
   only makes chat work over LSP via custom non-standard methods, inside
   clients GitHub also controls. That's evidence against, not for.

2. **Zed forces the decision.** Zed extensions are WASM (`wasm32-wasip2`)
   and the capability list is: languages/grammars, debuggers, MCP context
   servers, themes, icon themes, snippets. **No custom UI panels.** So the
   Neovim floating window has no Zed equivalent no matter what transport we
   pick. ACP sidesteps this entirely — register under `agent_servers` in
   `settings.json`, no WASM extension needed, Zed's Agent Panel renders it.

3. **Two protocols is the intended factoring, not a failure.** MCP answers
   "what tools can the agent reach" (client = the child AI CLI). ACP answers
   "where does the agent live in the editor" (client = the human). Different
   clients, different lifecycles. Both are JSON-RPC 2.0 descended from LSP,
   so the ACP code can reuse our existing MCP framing.

### Lifecycle mismatch — resolved

ACP/LSP/MCP servers are conventionally editor-spawned per workspace and die
with the window. Our daemon must outlive that (holds a warm child AI process
+ cross-session state). Answer: **thin editor-spawned adapter fronting a
persistent daemon.** This is what Copilot / Codeium / Supermaven / Tabnine
all do in some variant. Harden later into a systemd/launchd user service.

Rejected: making the editor-spawned server *be* the daemon (editors don't
reliably share one instance across windows/workspaces — you fight the client).

Also rejected: **pure plugin, no daemon** (the avante/codecompanion model).
Doesn't work for us because we spawn a child AI CLI, hold cross-session
state, and can't ship Lua UI to Zed. The daemon is justified.

### Migration stages

0. Factor the daemon's socket API into a clean internal contract. No
   behavior change; leave `POST /tutor` in place during transition.
1. Build `knumble-acp` in Go, reusing MCP's JSON-RPC layer. Implement
   `initialize`, `session/new`, `session/prompt`, `session/cancel`; emit
   `available_commands_update` and stream `session/update`.
   *Gate:* full round-trip against a scratch client.
2. Wire Neovim via codecompanion.nvim (fastest ACP client to configure).
   *Gate:* all five commands work + free-form Socratic Q&A streams.
3. Wire Zed via `agent_servers` in `settings.json`.
   *Gate:* conversation + `help`/`progress` render.
4. Daemon → user service; adapter auto-start/reconnect; delete the curl path
   and the floating window once at parity.

### Costs, explicitly

- Lose pixel-level control of the Neovim UI — the ACP client renders in
  *its* chat buffer, not ours. (Was going to lose this anyway, given Zed.)
- Depend on third-party Neovim ACP clients. Could ship a first-party minimal
  ACP client later if their UX drifts.
- ACP is young (created mid-2025). Betting on Zed + JetBrains backing it.
  Apache-2.0 but still Zed-steered, not vendor-neutral.
- One more process and two protocols to reason about.

### ⚠ Verify before writing code against these

The research flagged these as best-effort / inferred. Re-check at
implementation time.

**Checked at implementation (2026-08-21):** the ACP method names below are
correct as written -- `initialize`, `session/new`, `session/prompt`,
`session/cancel`, `session/update`, and `available_commands_update` all
round-tripped against a scratch client using `coder/acp-go-sdk` v0.13.5 at
protocol version 1. Notifications sent from inside a request handler are held until the
response is written, so the command menu can be published from `session/new`
itself.

- **ACP method names and message shapes** (`session/new`, `session/prompt`,
  `session/update`, `available_commands_update`) — spec is still evolving.
  stdio is the stable transport; Streamable HTTP is draft.
- **Copilot's chat method names** (`conversation/*`) — inferred from
  third-party integrations, not GitHub's official README. The load-bearing
  claim (chat uses custom non-standard methods) *is* directly supported.
- **All ACP adoption dates** (created Jun 2025, announced Aug 2025,
  JetBrains Oct/Dec 2025, registry Jan 2026, Zed 1.0 Apr 2026) — month-level
  precision is best-effort from blogs and tech press.
- **Zed's extension capability list** — Zed says it "plans on increasing the
  extension surface." If custom UI panels ever land, revisit stage 3.
- **Zed slash-command rendering for external agents** has reported rough
  edges. Fallback: commands still work as natural-language prompts, just
  without `/` autocomplete.

### Hard constraint to preserve

**Do not hand tutoring to the editor's built-in agent** (Zed's default
model, Copilot Chat, etc.). That discards the MENTOR.md + soul Socratic
contract, which is the core value. ACP lets our daemon plug in *as* the
agent, contract intact.
