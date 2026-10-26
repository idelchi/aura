---
layout: default
title: Automation
parent: Features
nav_order: 4
---

# Automation contract (API version 1)

This page is the normative description of Aura's machine contract: offline store operations through
`aura session … --json`, and live sessions served by `aura serve` under `/v1`. The wire types live in
`pkg/automation`; the Python client `aura_automation` (`clients/python`) implements them with strict models.

The contract is designed for unattended controllers that must survive their own restarts: every request is
idempotent by a client-chosen ID, every event is durably journaled and replayable, and a run is only complete once
the session file reflects it.

## Process model

- One `aura serve` process serves **one workspace and one live session**. To work on another session, start another
  process. Session-switching commands are rejected through the API.
- It runs the same engine path as the TUI, `aura run` and `aura web` (`RunSession` + the interactive loop); the API
  is only another UI.
- Store operations that must not load a session into an assistant (import, export, fork, freeze, show, list) are
  offline CLI commands.
- Prompts travel in HTTP request bodies, never in process arguments.

## Session store

The store is `<write home>/sessions`, where the write home is the **last** `--config` directory (else `--home`).
Pointing it outside a sandboxed workspace keeps sessions and journals out of the model's writable paths:

```sh
aura --home "" --config .aura --config /run/aura-home ...   # store: /run/aura-home/sessions
```

| Guarantee | Meaning |
| --- | --- |
| Atomic writes | A session file is replaced via a same-directory temporary file, fsync, rename and directory fsync. A crash leaves the previous or the new version, never a torn file. |
| Revision | `revision` is the sha256 of the stored file bytes. |
| Lease | A session is held by at most one process (flock on `sessions/.locks/<id>.lock`, released on exit). A second holder fails with `session_locked`. |
| Frozen | `frozen: true` is irreversible. A frozen session can be shown, exported and forked; resuming or saving it fails with `session_frozen` on every path. |
| Lineage | A fork stores `parent: {id, revision}` — the exact parent bytes it copied. The parent is only read. |
| Checkpoints | The live session is saved after every model iteration's tool results, after each turn (any outcome), after slash commands and at shutdown. Unchanged content is not rewritten. Saved history always holds complete call/result pairs. |

### `aura session`

```
aura [root flags] session list                                   [--json]
aura [root flags] session show   <id>                             [--json]
aura [root flags] session import <file> [--id <id>] [--freeze]    [--json]
aura [root flags] session export <id> --output <file>             [--json]
aura [root flags] session fork   <parent> --id <child> [--title <t>] [--expect-revision <sha256>] [--json]
aura [root flags] session freeze <id> [--expect-revision <sha256>] [--json]
```

With `--json`, stdout holds exactly one JSON object; failures print `{"error":{"code","message",...}}` on stderr and
exit 1.

| Command | Output |
| --- | --- |
| list | `{"sessions":[Summary],"warnings":[string]}` |
| show | `Summary` |
| import | `{"session":Summary,"created":bool,"frozen_now":bool,"source_sha256":string}` |
| export | `{"session":Summary,"path":string,"sha256":string,"written":bool}` |
| fork | `{"session":Summary,"parent":Summary,"created":bool}` |
| freeze | `{"session":Summary,"changed":bool}` |

`Summary = {id, title, created_at, updated_at, revision, messages, frozen, parent: {id, revision} | null}`, where
`messages` counts stored non-system messages.

Idempotency:

- **fork** — the child exists with lineage `{parent, current parent revision}`: `created: false`, returned as
  stored. Any other existing child: `exists`. The child's non-system history equals the parent's (verified).
- **import** — the target is absent: stored, `created: true`. Present with the same conversation (title, messages,
  metadata, todos; timestamps, freezing and lineage ignored): `created: false` (frozen now if `--freeze`). A
  different conversation: `exists`. `source_sha256` is the input file's digest; the stored bytes are Aura's encoding,
  so pin the returned `revision`.
- **export** — writes the stored bytes verbatim; an identical existing file: `written: false`; a different file:
  `exists`.
- **freeze** — already frozen: `changed: false`.

Codes: `not_found`, `exists`, `revision_mismatch`, `session_locked`, `session_frozen`, `invalid_id`,
`invalid_session`, `bad_request`.

## `aura serve`

```sh
AURA_SERVE_TOKEN=<at least 32 characters> \
  aura [root flags] [--resume <id>] serve [--bind 127.0.0.1:0] --ready-file <path>
```

- The bearer token is read from `AURA_SERVE_TOKEN` and **removed from the environment before any tool, LSP or MCP
  child starts**; it is never written to disk.
- Without `--resume`, a fresh session with a UUID is created and saved before readiness.
- The ready file (written atomically, mode 0600) appears only after the configuration is loaded, the session is
  resumed or created and saved, every `required` MCP server is connected, the journal is opened and recovered, and
  the listener is up:

  ```json
  {"api":1,"url":"http://127.0.0.1:PORT","pid":123,"session_id":"…","aura_version":"…","commit":"…","workdir":"…","write_home":"…"}
  ```

  On failure the process exits 1 after writing `{"api":1,"error":{"code","message","retryable","details"}}`.
  Startup codes: `unauthorized` (token), `session_frozen`, `session_locked`, `not_found`, `invalid_id`,
  `resume_failed`, `mcp_unavailable`, `config_invalid`, `startup_failed`, and the provider codes below (with
  `details`) when a startup step that contacts the provider fails — for example restoring a model when `--model`
  changes the stored selection. A transient outage therefore stays `retryable` and distinct from configuration or
  credential faults. Serve does not otherwise contact the provider before the first run.
- Shutdown on SIGINT, SIGTERM or `POST /v1/shutdown`: new runs are refused (`shutting_down`), the active run is
  cancelled with reason `shutdown` and up to 60 s are given for it to settle, then the server exits 0 (1 if the final
  save failed). A run that does not settle is reported `lost` by the next server on that session. Pending approvals
  are denied and questions dismissed — automation never approves on its own.

## HTTP API

Every request needs `Authorization: Bearer <token>`. Bodies are JSON; unknown fields are rejected. Errors are
`{"error":{"code","message","retryable","details"}}`.

| Method and path | Body / query | Success |
| --- | --- | --- |
| `GET /v1/server` | | `ServerInfo` |
| `GET /v1/session` | | `SessionInfo` (stored summary + `persisted_seq`, live `meta`, `status`) |
| `GET /v1/session/messages` | `after` (index), `thinking` (default false) | `{"messages":[…],"total":N}` |
| `POST /v1/runs` | `{"request_id","prompt"}` or `{"request_id","command"}`, optional `"on_input":"block"\|"wait"` (default `block`) | `202 Run`; `200 Run` with `Idempotent-Replay: true` |
| `GET /v1/runs/{id}` | | `Run` |
| `GET /v1/runs?request_id=` | | `Run` |
| `POST /v1/runs/{id}/cancel` | | `202 Run` |
| `POST /v1/runs/{id}/inputs/{input_id}` | `{"answer"}` (ask) or `{"action":"allow"\|"allow_session"\|"deny"}` (confirm) | `200 {}` |
| `GET /v1/journal` | `after` (seq), `limit` (1–5000, default 500), `wait` (0–60 s long-poll) | `{"events":[Event],"last_seq":N}` |
| `GET /v1/events` | SSE; `after` or `Last-Event-ID`, `transient`, `thinking`, `heartbeat` (1–60 s, default 15) | stream |
| `POST /v1/shutdown` | | `202 {}` |

Request failures: `400 bad_request`, `401 unauthorized`, `404 not_found`, `409 busy` (another run has not settled),
`409 request_conflict` (same `request_id`, different body), `409 cursor_out_of_range` (`after` < 0 or > last seq;
`details.last_seq`), `409 not_pending`, `503 shutting_down`.

A prompt is always a user message, even when it starts with `/`. A command must start with `/` and name a known
slash command; commands that change the served session (`/resume`, `/load`, `/fork`, `/name`, `/clear`, `/new`,
`/exit`, `/quit`) are rejected with `bad_request`.

## Runs

```
Run {
  id: "run_<uuidv7>", request_id, kind: "prompt"|"command", request_digest, on_input: "block"|"wait", session_id,
  state: "accepted"|"running"|"finished"|"persisted"|"persist_failed"|"lost",
  terminal: bool,                              # state ∈ {persisted, persist_failed, lost}
  outcome: null|"completed"|"failed"|"cancelled"|"rejected"|"blocked",
  error: null|Error,                           # null iff outcome is null or completed
  persist_error: null|Error,
  cancel_requested: bool,
  result: null|{"content"},                    # final assistant text, or the command's output
  persisted: null|{"revision","messages","seq"},
  stats: {"iterations","tool_calls":{name:count},"tool_errors"},
  accepted_seq, accepted_at, started_at|null, finished_at|null, settled_at|null
}
Error { code, message, retryable, details: {status?, provider?, retry_after_ms?, reason?, last_seq?} }
```

State machine: `accepted → running → finished → persisted | persist_failed`; `accepted → finished` for rejected
inputs and commands; any unsettled run becomes `lost` when the next server starts on the session. **A client is
done only when `terminal` is true.** `finished` means the model stopped, not that the session is safe.

- `persisted` is set only after the session save returned successfully; `persisted.revision` is the stored file's
  sha256. While a run is in flight, `persisted` describes its latest checkpoint.
- `persist_failed` sets `persist_error` (code `persist_failed`) and keeps the outcome.
- `lost`: `outcome` stays as recorded (null if the run never finished), `error` is `server_lost` if it never
  finished (else `persist_error` is), and `persisted` describes the session file found by the next server.
- A rejected run adds no message to the stored conversation (metadata such as statistics may still be saved).

Outcomes and codes:

| Outcome | Codes |
| --- | --- |
| completed | — |
| cancelled | `cancelled` (`details.reason`: `request` or `shutdown`) |
| rejected | `input_rejected` (no context room, or empty after directive expansion), `guardrail_blocked` |
| blocked | `input_required` (ask or confirm under `on_input: block`), `policy_stop` (plugin `Stop`) |
| failed | `provider_network`, `provider_server`, `provider_rate_limit`, `provider_auth`, `provider_credit`, `provider_model_unavailable`, `content_filter`, `context_exhausted`, `compaction_failed`, `max_steps`, `token_budget`, `command_failed`, `persist_failed` (a mid-run checkpoint), `internal` |

`retryable` is true only for `provider_network`, `provider_server` and `provider_rate_limit`. The provider's HTTP
status is in `details.status` when known, its message in `message` (for example a llama-swap 503 "the engine is not
running"), and the retry hint in `details.retry_after_ms`.

### Idempotency

`request_id` is unique within a session's journal, across server restarts. The same `request_id` with the same
`{kind, text, on_input}` returns the current record in any state and never executes again; a different body is
`request_conflict`. Recover a `lost` run by submitting a new request (for example a continuation prompt).

### Cancellation

`POST …/cancel` records `cancel_requested` (journaled as `run.cancel_requested`) and signals the engine; it is a
request, not a result. The terminal record is authoritative: `cancelled` if the cancellation took effect, otherwise
the natural outcome. A cancelled turn stops at the next iteration boundary; in-flight tool calls receive error
results (an MCP call also sends `notifications/cancelled` for its request), the session is saved, and the run
settles. Cancelling a settled run returns it unchanged.

### Questions and approvals

With `on_input: block`, a question (Ask tool) or approval (tool policy `confirm`) cancels the run, which ends
`blocked` with `input_required` — it never waits. With `on_input: wait`, the server journals `input.required` and the
run waits for `POST …/inputs/{input_id}`. An ask's `answer` may be an option label or its 1-based number.

## Journal and events

`<store>/journal/<session-id>.jsonl` holds one JSON object per line:

```json
{"seq":12,"time":"…","type":"tool.finished","session_id":"…","run_id":"run_…","data":{…}}
```

- `seq` starts at 1, is contiguous and continues across server restarts; `run_id` is null outside runs.
- Each event is appended with one write; lifecycle events are fsynced. A crash can only leave a torn final line,
  which the next server truncates (reporting a `notice` from source `journal`); such a line was never served. Any
  other invalid line is a hard error.
- Events never contain thinking. Forks do not copy journals.
- Readers pull from the journal, so a slow client never loses events.

Durable event types and data:

| Type | Data |
| --- | --- |
| `server.started` | `{pid, version, resumed_revision}` |
| `server.stopping` | `{reason}` |
| `run.accepted`, `run.cancel_requested`, `run.finished`, `run.persisted`, `run.persist_failed`, `run.lost` | `{run: Run}` after the transition |
| `run.started` | `{run_id, message_id}` — the input entered the conversation |
| `message.user` | `{message_id, content}` |
| `message.assistant` | `{message_id, content, tool_calls: [{call_id, name, args}]}` — one per model iteration |
| `tool.started` | `{call_id, name, args}` |
| `tool.finished` | `{call_id, name, status: "ok"\|"error", result, error}` — `result` is what entered the conversation |
| `notice` | `{level, source, message}` (warnings, plugin injections, failover) |
| `status` | `{agent, mode, provider, model, iteration, tokens_used, tokens_max}`, only when it changes |
| `input.required` | `{input_id, kind: "ask"\|"confirm", question, options: [{label, description}], multi_select, tool, detail, pattern}` |
| `input.resolved` | `{input_id, answer}` |
| `command.result` | `{command, message, error}` |
| `session.persisted` | `{revision, messages, reason: "start"\|"checkpoint"\|"run"\|"command"\|"shutdown", changed}` |
| `session.persist_failed` | `{error: Error, reason}` |

Order within a prompt run: `run.accepted → message.user → run.started → (message.assistant → tool.started… →
tool.finished… → session.persisted[checkpoint])… → message.assistant[final] → run.finished →
session.persisted[run] → run.persisted`.

### Streaming

`GET /v1/journal?after=&wait=` long-polls and is the stop-aware primitive. `GET /v1/events` streams the same events
as SSE (`id: <seq>`, `event: <type>`, `data: <Event>`), replaying from `after` or `Last-Event-ID`. A quiet stream
carries `event: heartbeat` with `data: {}` every `heartbeat` seconds.

With `transient=true` the stream also carries unsequenced live events without an `id` field: `delta.content`
`{message_id, text}`, `delta.thinking` (only with `thinking=true`), `tool.output` `{name, line}` and `spinner`
`{text}`. They may be dropped and are superseded by durable events. Because the SSE last-event-ID is sticky, clients
recognise them by event type.

## MCP servers in automation

- `required: true` — a server that fails to start, initialize or list its tools fails startup on every path (serve:
  `mcp_unavailable`, no ready address).
- `sandbox: true` (stdio only) — confines the server with the session's effective sandbox restrictions; see
  [Sandboxing]({{ site.baseurl }}/features/sandboxing#mcp-servers).
- A result with `isError: true` is a tool error (`tool.finished.status = "error"`, its text in `error`).
- Run cancellation sends `notifications/cancelled` for the in-flight request.

See [MCP]({{ site.baseurl }}/features/mcp).

## Python client

`clients/python` builds the `aura-automation` wheel. Every response is validated by strict models, so drift between
the client and the binary fails loudly; pin both to the same commit.

```python
from aura_automation import Aura, Server, Store

aura = Aura(binary=binary, args=["--home", "", "--config", ".aura", "--config", str(home)], cwd=workspace, env=env)
Store(aura).fork("baseline", "task-1")
handle = Server.launch(aura, ready_file=ready, token=token, resume="task-1")
with handle.client() as client:
    run = client.submit("task-1:author:1", prompt=assignment)
    final = client.wait(run.id, on_event=mirror, stop=should_stop)
handle.stop()
```

`Client.wait` long-polls the journal, calls `on_event` for every durable event in order, honours `stop` within its
poll interval (raising `StopRequested` without cancelling) and returns the terminal run. `Server.attach` reconnects to
a running server after a controller restart. The client's end-to-end tests drive the real binary with the
[script provider]({{ site.baseurl }}/configuration/providers#script).

## `aura web` JSON endpoints

`aura web` keeps its `/send`, `/command`, `/cancel`, `/ask`, `/confirm` and `/events/json` endpoints. They are a UI
event interface for the browser and quick interactive testing: events are not sequenced, slow readers lose events,
input sent during a turn merges into the next one and prompts are not correlated. Use `aura serve` for automation.
