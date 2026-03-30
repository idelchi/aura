---
layout: default
title: Sessions
parent: Features
nav_order: 3
---

# Session Persistence

Save, resume, and fork conversations as JSON snapshots.

## Storage

Sessions are stored at `.aura/sessions/{id}.json`. The ID is either a UUID (auto-generated) or a user-chosen name set via `/name`. Each session contains:

- Conversation history (all messages except ephemeral error feedback)
- Metadata: title, agent, mode, model, provider, thinking level, thinking display state, loaded tools (deferred tools activated via LoadTools), sandbox state, read-before policy, session approvals, stats, cumulative token usage
- Todo list

## Commands

| Command         | Description                                                         |
| --------------- | ------------------------------------------------------------------- |
| `/save [title]` | Save current session. Preserves existing title on re-save.          |
| `/name [name]`  | Set or show the session name (custom ID). No args shows current ID. |
| `/resume [id]`  | List sessions or resume by ID prefix match.                         |
| `/load`         | Alias for `/resume`.                                                |
| `/fork [title]` | Fork into new session. Auto-titles as "Fork of {original}".         |

## Auto-Save

Sessions are automatically saved when Aura exits, but only if at least one user message was sent during the session. Starting Aura and immediately quitting does not create an empty session file.

## Resume

Resuming restores agent, mode, model/provider, thinking level, messages, todos, loaded tools, sandbox state, read-before policy, session approvals, and stats. The TUI replays the conversation filtered for display (user messages, assistant responses, tool calls, DisplayOnly notices, Bookmark dividers); system messages and raw tool results are excluded.

Use `/resume` without arguments to open the interactive session picker (TUI) or list sessions (Simple/Headless).

### CLI Flags

Resume a specific session on startup with `--resume`:

```sh
aura --resume abc123
```

Accepts the same ID prefix as `/resume`. Supports `AURA_RESUME` environment variable.

Resume the most recently updated session with `--continue` (`-c`):

```sh
aura --continue
aura -c
```

Picks the session with the latest `UpdatedAt`. Cannot be combined with `--resume`.

## Named Sessions

By default, sessions get a UUID as their ID. Use `/name` to assign a human-friendly name:

```
> /name weekly-review
Session named: weekly-review

> /save
Saved session: weekly-review (Weekly Code Review)

> /resume weekly
Resumed session: weekly-review (Weekly Code Review)
```

- `/name weekly review` (multiple words) → `weekly-review` (joined with dashes)
- Names must be unique — error if another session already has that name
- Works before or after saving — set the name first, then `/save`, or rename an already-saved session
- `/resume` prefix matching works identically for UUIDs and custom names

## Title Generation

Session titles are automatically generated using a dedicated Title agent when saving. Configure in `.aura/config/features/title.yaml`:

- `disabled: true` — skip LLM generation, use first user message instead
- `agent: "Title"` — dedicated agent for title generation
- `prompt: ""` — named prompt for self-title (overrides agent — uses current model)
- `max_length: 50` — maximum character length

Title uses the same `ResolveAgent()` framework as Compaction and Thinking. If neither `agent` nor `prompt` is set, falls back to the first user message.

## Forking

`/fork` copies the current session into a new one (fresh UUID, title "Fork of {original}"). Both sessions are independent after forking. Use `/name` afterward to rename the fork.
