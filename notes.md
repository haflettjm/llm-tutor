# Notes

Scratch notes and decision records. Not polished docs — move things into
`docs/` once they settle.

---

## 2026-08-17 — Editor integration: adopt ACP, keep the daemon

**Status:** research done, not yet decided-decided. Nothing implemented.
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
implementation time:

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
