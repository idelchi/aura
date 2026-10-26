---
layout: default
title: Sessions
parent: Features
nav_order: 3
---

# Session Persistence

Save, resume, and fork conversations as JSON snapshots.

## Git Snapshots and Rewind

Before each accepted user prompt in a Git repository, Aura captures the working
tree for `/undo` (`/rewind`). These temporary Git refs are separate from saved
conversation sessions. `/undo` can restore code, rewind messages, or do both.

To disable automatic code snapshots, add `.aura/config/features/snapshot.yaml`:

```yaml
snapshot:
  disabled: true
```

This avoids snapshot-related Git work and leaves message-only rewind available.
Existing snapshots from earlier enabled turns are cleaned up by `/clear` (`/new`)
or when the session closes; changing this setting does not delete saved conversation sessions.
Agent, mode, task, and CLI overrides use the same [feature setting]({{ site.baseurl }}/configuration/features#git-snapshots).

## Storage

Sessions are stored at `<write home>/sessions/{id}.json`, where the write home is the last `--config` directory (by default the project's `.aura`). The ID is either a UUID (auto-generated) or a user-chosen name set via `/name` (letters, digits, `.`, `_`, `-`). Each session contains:

- Conversation history (all messages except ephemeral error feedback, which is dropped together with the tool calls it answered, so a saved history only holds complete call/result pairs)
- Metadata: title, agent, mode, model, provider, thinking level, thinking display state, loaded tools (deferred tools activated via LoadTools), sandbox state, read-before policy, session approvals, stats, cumulative token usage
- Todo list
- `frozen` (immutable sessions) and `parent` (`{id, revision}` of the stored parent a fork was copied from)

Writes are atomic: the file is replaced via a temporary file, fsync and rename, so a crash leaves either the previous or the new version. A session's `revision` is the sha256 of its stored bytes.

A session is held by one process at a time (a lock under `sessions/.locks/`, released when the process exits). Opening a session another process holds fails with "session is in use by another process".

## Commands

| Command         | Description                                                         |
| --------------- | ------------------------------------------------------------------- |
| `/save [title]` | Save current session. Preserves existing title on re-save.          |
| `/name [name]`  | Set or show the session name (custom ID). No args shows current ID. |
| `/resume [id]`  | List sessions or resume by ID (an exact ID wins over prefix matches). |
| `/load`         | Alias for `/resume`.                                                |
| `/fork [title]` | Fork into new session. Auto-titles as "Fork of {original}".         |
| `/clear`, `/new` | Start a new conversation. The current session stays as last saved; the next save creates a new session. |

## Auto-Save

Once at least one user message was accepted into the conversation (or a session was resumed), the session is saved automatically:

- after every model iteration's tool results (a checkpoint: a crash or kill loses at most the iteration in progress),
- after every turn, whatever its outcome (completed, failed, cancelled),
- after slash commands, and
- when Aura exits.

Content that already matches the stored revision is not rewritten. This applies to the interactive UI, `aura web`, `aura run`, tasks and `aura serve` alike, without `/save`. A run that fails after accepting input still has the history it reached, so it can be continued with `--continue` or `--resume`.

Starting Aura and immediately quitting does not create an empty session file. Neither do blank input, input rejected before it enters the conversation, or slash commands that send nothing to the model. (`aura serve` saves a fresh session immediately, so its ID is durable before it is announced.)

## Resume

Resuming restores agent, mode, model/provider, thinking level, messages, todos, loaded tools, sandbox state, read-before policy, session approvals, and stats. Every part must restore: if the stored agent, mode, model, read-before policy or sandbox cannot be applied (for example a removed agent, or a sandboxed session on a kernel without Landlock), resuming fails instead of continuing with different settings. A failed `/resume` leaves a fresh, unsaved conversation, so the stored session is never overwritten by a half-restored one.

Resuming another session inside a running Aura first resets everything session-scoped — conversation, file reads (read-before), loaded deferred tools, todos, approvals, statistics and tool budgets — so nothing carries over from the previous session.

Frozen sessions cannot be resumed; fork them first (see below). The TUI replays the conversation filtered for display (user messages, assistant responses, tool calls, DisplayOnly notices, Bookmark dividers); system messages and raw tool results are excluded.

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

`/fork` copies the current session into a new one (fresh UUID, title "Fork of {original}") and continues in the copy. Both sessions are independent after forking. Use `/name` afterward to rename the fork.

## Store Operations

`aura session` operates on stored sessions without starting a model:

```sh
aura session list
aura session show <id>
aura session import <file> [--id <id>] [--freeze]   # bring a native session file into the store
aura session export <id> --output <file>
aura session fork <parent> --id <child> [--title <t>] [--expect-revision <sha256>]
aura session freeze <id> [--expect-revision <sha256>]
```

- **Freeze** makes a session permanently immutable. It can still be shown, exported and forked — the way to keep a
  reviewed conversation (a teaching baseline) unchanged while reusing it.
- **Fork** stores an independent copy and records the parent's exact revision. The parent file is only read, never
  written, so frozen and live parents alike stay byte-for-byte unchanged.
- Import, export, fork and freeze are idempotent; repeating one with the same inputs reports that nothing changed.

Add `--json` for machine-readable output; the [automation contract]({{ site.baseurl }}/features/automation#session-store)
specifies every field and error code.
