# LLM Programming Tutor

A Socratic programming tutor for Neovim (Zed planned). Teaches any language by asking questions rather than supplying answers.

```text
Editor plugin ── HTTP / Unix socket ── Go backend
(chat panel)                           ├── MENTOR.md  (universal teaching contract)
                                       ├── souls/     (role-specific tutor personas)
                                       ├── lesson-plans/
                                       └── ~/.local/share/llm-tutor/
                                           ├── learning-events.jsonl
                                           └── progress.json
```

The editor plugin attaches the current git diff and detected language to every message. The backend selects the right soul file, composes the system prompt, and calls the Anthropic API.

## Setup

```bash
export ANTHROPIC_API_KEY=your_key_here
make run
```

## Lesson plans

- [Go Fundamentals](lesson-plans/go-fundamentals.md) — 10 concepts, entry level
- [Python Foundations](lesson-plans/python-foundations.md) — 14 modules, entry level

## Status

Early scaffold. See the product plan in the Obsidian vault for the roadmap.
