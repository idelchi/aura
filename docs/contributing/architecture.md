---
layout: default
title: Architecture
parent: Contributing
nav_order: 1
---

{% raw %}

# Architecture

## Core Flow

Every interaction follows this lifecycle:

```
User input → Parse directives (@Image, @File, @Path, @Bash)
  → Assistant loop (goroutine):
    → Auto-compact if thresholds hit
    → Build conversation (Builder)
    → Send to LLM provider (Chat)
    → Stream response back (Events → UI)
    → Execute tool calls (built-in + plugin)
    → Inject guardrail/hook messages
    → Repeat until no tool calls remain (or Done tool fires)
  → Drain pending inputs
  → Wait for next input
```

The assistant runs in a goroutine. User input queues into `pendingInputs` while the LLM is streaming. `Ctrl+C` cancels the current stream. After each stream completes, all queued inputs drain before the loop waits again.

## Key Types

**Assistant** (`internal/assistant/`) — the main loop. ~45 fields organized into inner structs by lifecycle phase:

- `loopState` — per-turn state (iteration counter, tool history, pending ejects)
- `resolvedState` — recomputed on agent/mode/config change (resolved model, tools, sandbox, hooks, guardrail)
- `sessionState` — session lifecycle (manager, stats, usage, approvals)
- `streamState` — streaming coordination (active flag, pending queue)

**Builder** (`internal/conversation/`) — conversation history and event emission. Maintains the message array, streams UI events via an `EventSink`, and manages the current message cursor.

**Provider** (`pkg/providers/`) — 4-method core interface:

```go
Chat(ctx, request, streamFunc) (message, usage, error)
Models(ctx) (models, error)
Model(ctx, name) (model, error)
Estimate(ctx, request, text) (int, error)
```

Optional capabilities use opt-in interfaces (same pattern as tools): `Embedder`, `Reranker`, `Transcriber`, `Synthesizer`, `ModelLoader`. Callers discover capabilities via `providers.As[T](provider)` which unwraps wrappers like `RetryProvider`.

**Tool** (`pkg/llm/tool/`) — 7 core methods + 9 opt-in interfaces:

- Core: `Name`, `Description`, `Usage`, `Examples`, `Schema`, `Execute`, `Available`
- Opt-in (type-asserted at call sites): `PreHook`, `PostHook`, `PathDeclarer`, `Previewer`, `SandboxOverride`, `ParallelOverride`, `LSPAware`, `Overrider`, `Closer`
- All tools embed `tool.Base`, which provides no-op defaults for `Pre`/`Post`/`Paths`

**Config** (`internal/config/`) — selective loading via `config.New(opts, parts...)` returning `(Config, Paths, error)`. `Config` holds loaded YAML (immutable); `Paths` holds filesystem context; `Runtime` (created by the caller) holds CLI flags and computed state. Callers request only the parts they need (15 `Part` constants from `PartModes` through `PartApprovalRules`). Two generic collection types:

- `Collection[T]` — file-keyed entities (agents, modes, systems, skills, commands)
- `StringCollection[T]` — string-keyed entities (providers, MCPs, plugins)

**Injector** (`internal/injector/`) — plugin hooks produce typed injection values dispatched by timing. A base `Injection` struct holds 7 universal fields (Name, Role, Content, Prefix, Eject, Tools, DisplayOnly). Five timing-specific structs embed it, each carrying one modification field (`BeforeChatInjection.Request`, `AfterResponseInjection.Response`, `AfterToolInjection.Output`, `OnErrorInjection.Error`, `BeforeCompactionInjection.Compaction`). The `Registry` groups injectors by timing and runs them via typed `RunXxx()` methods. Generic timings (AfterCompaction, OnAgentSwitch) use `Run()` which returns `[]Injection` directly.

## Package Layers

| Layer     | Packages                                                                              | Purpose                |
| --------- | ------------------------------------------------------------------------------------- | ---------------------- |
| CLI       | `internal/cli/`, `internal/cli/core/`, `internal/cli/packs/` + 14 subcommand packages | urfave/cli v3 commands |
| Core      | `internal/assistant/`, `internal/conversation/`, `internal/config/`                   | Loop, history, config  |
| Tools     | `internal/tools/` (24 tools), `internal/plugins/`, `internal/mcp/`                    | Tool execution         |
| Providers | `pkg/providers/` (8 implementations)                                                  | LLM backends           |
| Domain    | `pkg/llm/` (message, model, tool, usage + 11 more subpackages)                        | Shared types           |
| UI        | `internal/ui/` (tui, simple, headless, web)                                           | 4 UI backends          |
| SDK       | `sdk/` (separate Go module)                                                           | Plugin contract        |

## Config System

**Selective loading** — `config.New(opts, parts...)` returns `(Config, Paths, error)` and only loads requested parts. The CLI loads everything; subcommands load subsets. Each `Part` constant maps to a loader function in a registry.

**Two parsing paths:**

- Markdown entities (agents, modes, systems, skills, commands) → `frontmatter.LoadRaw()` extracts YAML frontmatter + body
- Pure YAML entities (providers, features, hooks, tasks, mcp, lsp, sandbox, tools, approval-rules) → `yamlutil.StrictUnmarshal()` with unknown-field rejection

**Multi-home merging** — `--config a,b,c` merges config directories left-to-right (last wins). Project `.aura/config/` and global `~/.aura/config/` merge automatically.

**Entity inheritance** — `inherit: [parent]` with DAG cycle detection (`internal/config/inherit/`). Children override parent fields via `merge.Merge()`.

**Template rendering** — Go templates + slim-sprig in all prompts. Template variables include `{{ .Agent }}`, `{{ .Mode.Name }}`, `{{ .Provider }}`, and user-defined `{{ .Vars }}` via `--set`.

## Tool System

**Registration** — tools register in `internal/tools/tools.go` (`All()` function). Tool assembly (base tools + Task + Batch + ToolDefs) is handled by `internal/tools/assemble` (`assemble.Tools()`), called from CLI inspection, session init, and per-turn config refresh.

**Base embedding** — every tool struct embeds `tool.Base`, which provides no-op implementations of `Pre`, `Post`, and `Paths`. Override only the methods that have real logic.

**Schema generation** — `Schema()` returns a `tool.Schema` with name, description, and JSON Schema parameters. The schema is sent to the LLM for function calling.

**Two execution paths:**

1. **Direct (in-process)** — tool's `Execute()` runs in the assistant goroutine. Full access to Go context, working directory, and SDK context.
2. **Sandboxed (re-exec)** — parent forks a child process via `aura tools --json --raw -H <name> <argsJSON>`. The child reads SDK context from stdin, executes the tool under Landlock LSM restrictions, and returns JSON on stdout. Streaming uses `\x00STREAM:` prefix on stderr.

**Opt-in interfaces** — tools declare capabilities by implementing opt-in interfaces. The assistant type-asserts at call sites (not on Base). Defaults are at call sites: sandboxable=true, parallel=true.

**Tool definitions** — YAML overrides in `.aura/config/tools/` can change a tool's description, usage, examples, parallel behavior, disabled state, and conditional inclusion.

## Provider System

Eight user-facing implementations under `pkg/providers/`: `ollama`, `llamacpp`, `openrouter`, `openai`, `anthropic`, `google`, `copilot`, `codex`. A `noop` provider is also available for testing and `--dry=noop` mode.

**Factory** (`internal/providers/factory.go`) — creates provider instances from config. Fantasy-based providers (Anthropic, OpenAI, Google, OpenRouter, Copilot, Codex) have built-in retry via `charm.land/fantasy`. Native providers (Ollama, LlamaCPP) use per-provider `retry:` config wrapped with `RetryProvider` for exponential backoff.

**Fantasy adapter** (`pkg/providers/adapter/`) — bridges Aura's LLM types with Fantasy's unified provider abstraction. Six providers delegate Chat() through Fantasy — the adapter handles message conversion (`ToCall`), streaming (`StreamToMessage`), and error mapping (`MapError`). Non-chat methods (Embed, Rerank, Transcribe, Synthesize) stay native SDK.

**Opt-in interfaces** — providers implement only the capabilities they support (Embedder, Reranker, Transcriber, Synthesizer, ModelLoader). Callers use `providers.As[T](provider)` to discover capabilities through wrapper layers.

**Error handling** — Fantasy-based providers use `adapter.MapError()` to convert Fantasy errors into typed Aura errors (rate limit, auth, context length, etc.). Native providers (Ollama, LlamaCPP) use `handleError()` + `providers.ClassifyHTTPError()` for the same purpose.

## Injector System

**9 timings** — `BeforeChat`, `AfterResponse`, `BeforeToolExecution`, `AfterToolExecution`, `OnError`, `BeforeCompaction`, `AfterCompaction`, `OnAgentSwitch`, `TransformMessages`.

**Type system** — base `Injection` struct (7 universal fields) + 5 typed structs that embed it, each carrying one timing-specific modification field. Invalid combinations (e.g., `Output` on a BeforeChat injection) are prevented at the type level. A `HasBase` interface + `Bases[T]()` generic bridges typed slices to `[]Injection` for functions that only need base fields (e.g., `injectMessages`).

**Registry** (`internal/injector/`) — grouped by timing. A generic `runCheckers[C, R]` helper powers 5 typed `RunXxx()` one-liners (`RunBeforeChat`, `RunAfterResponse`, `RunAfterTool`, `RunOnError`, `RunBeforeCompaction`) that return typed slices. `castTo[C]` handles the type assertion. Generic `Run()` returns `[]Injection` for AfterCompaction and OnAgentSwitch. Two specialized methods with different signatures: `RunBeforeTool` (argument chaining) and `RunTransformMessages` (message pipeline).

**Checker interfaces** — each typed timing has a checker interface (`BeforeChatChecker`, `AfterResponseChecker`, etc.) that `Hook` implements. The `runCheckers` helper type-asserts to these interfaces via `castTo`, so only hooks with the correct timing are dispatched.

**Plugin dispatch** (`internal/plugins/dispatch.go`) — `timingRegistry` maps each timing to `assign` + `dispatch` functions. Plugin hooks are Go functions loaded via Yaegi. Each timing has a corresponding SDK context type (e.g., `sdk.BeforeToolContext`) passed to the plugin function.

**Hook struct** (`internal/plugins/hook.go`) — one function field per timing. `Check()` only handles generic timings (AfterCompaction, OnAgentSwitch). All other timings use typed `CheckXxx()` methods called by the registry's typed `RunXxx()`. Common preamble (once-check, condition, env, dispatch) is shared via `prepareAndDispatch`.

## Message Types

Six message types control visibility across the system:

| Type        | Sent to LLM | Persisted | Rendered   | Exported  |
| ----------- | ----------- | --------- | ---------- | --------- |
| Normal      | Yes         | Yes       | Yes        | Yes       |
| Synthetic   | One turn    | No        | Yes        | No        |
| Ephemeral   | One turn    | No        | Yes        | No        |
| DisplayOnly | Never       | Yes       | Yes (text) | Text only |
| Bookmark    | Never       | Yes       | Divider    | Text only |
| Metadata    | Never       | Yes       | No         | JSON only |

Intent-based query methods on the message collection (`ForLLM()`, `ForCompaction()`, `ForSave()`, `ForExport()`, `ForDisplay()`) encapsulate filtering logic so consumers express what they need, not what to exclude.

## Event System

The assistant communicates with the UI through a typed event channel (buffered, 100 capacity). Each event implements `Tag() EventTag`, grouping 21 event types into 6 semantic categories:

| Tag          | Events                                                                                                                                     |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `TagMessage` | `MessageAdded`, `MessageStarted`, `MessagePartAdded`, `MessagePartUpdated`, `MessageFinalized`                                             |
| `TagStatus`  | `StatusChanged`, `DisplayHintsChanged`, `AssistantDone`, `WaitingForInput`, `UserMessagesProcessed`, `SpinnerMessage`, `SyntheticInjected` |
| `TagTool`    | `ToolOutputDelta`, `ToolConfirmRequired`                                                                                                   |
| `TagDialog`  | `AskRequired`, `TodoEditRequested`                                                                                                         |
| `TagSession` | `SessionRestored`, `SlashCommandHandled`, `CommandResult`                                                                                  |
| `TagControl` | `PickerOpen`, `Flush`                                                                                                                      |

Four UI backends consume these events: TUI (Bubble Tea), Simple (readline), Headless (no display), Web (htmx + SSE).

{% endraw %}
