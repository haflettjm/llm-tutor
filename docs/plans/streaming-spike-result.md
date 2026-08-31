# Streaming Spike Result: `--json-schema` with `stream-json`

## Verdict: A, deltas are JSON fragments

The CLI accepted `--json-schema` with `--output-format stream-json` and exited successfully. It did not emit `text_delta` events for the structured response. Instead, the model returned the structured object through a `tool_use` block, with fragments at `event.delta.partial_json` on `stream_event` records where `event.type` is `content_block_delta` and `event.delta.type` is `input_json_delta`.

The learner-visible message is therefore JSON source while it streams. Task 4 is required, but Task 3 must read `input_json_delta.partial_json` rather than `text_delta.text`.

## Capture

Captured on 2026-08-31 with Claude Code 2.1.251:

```text
printf 'Ask me one short Socratic question about loops.' |
  claude -p --verbose --safe-mode --tools '' --strict-mcp-config \
    --no-session-persistence \
    --output-format stream-json \
    --include-partial-messages \
    --json-schema '<response schema>'
```

`--safe-mode`, an empty tool list, strict MCP configuration, and a temporary directory kept the probe independent of project hooks and MCP servers.

## Event counts

Top-level event types:

```text
assistant: 2
rate_limit_event: 1
result (success): 1
stream_event: 16
system (init): 1
system (status): 1
system (thinking_tokens): 2
user: 1
```

`stream_event.event.type` counts:

```text
content_block_delta: 9
content_block_start: 2
content_block_stop: 2
message_delta: 1
message_start: 1
message_stop: 1
```

`content_block_delta.event.delta.type` counts:

```text
input_json_delta: 6
thinking_delta: 2
signature_delta: 1
```

There were zero `text_delta` events and zero `delta.text` values. The six `input_json_delta` events carry raw fragments at `event.delta.partial_json`.

## Final envelope

The final `type: "result", subtype: "success"` envelope was present. Its `result` was a JSON-encoded response object, and its `structured_output` held the decoded object:

```json
{
  "message": "When you write a loop, what has to change on each pass for it to ever stop?",
  "response_type": "question",
  "hint_level": 0,
  "concept_id": "loops.termination"
}
```

The existing `parseClaudeOutput` supports this result shape because it already falls back from a direct object to a JSON-encoded result string.

## Decision for implementation

- Keep `--json-schema` for streaming turns.
- Decode `stream_event.event.delta.partial_json` only when `event.delta.type` is `input_json_delta`.
- Feed those fragments to the Task 4 message-field scanner before publishing learner-visible chunks.
- Ignore `thinking_delta`, `signature_delta`, lifecycle events, and unknown event types.
