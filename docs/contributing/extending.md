---
layout: default
title: Extending
parent: Contributing
nav_order: 2
---

{% raw %}

# Extending Aura

Step-by-step guides for adding new capabilities. Each guide lists every file that needs changes.

## Adding a New Built-in Tool

### 1. Create the tool package

Create `internal/tools/<name>/<name>.go`:

```go
package <name>

import (
    "context"

    "github.com/idelchi/aura/pkg/llm/tool"
)

type Tool struct {
    tool.Base
}

func New() *Tool {
    return &Tool{
        Base: tool.Base{
            Text: tool.Text{
                // Description, Usage, Examples set here or via YAML overrides
            },
        },
    }
}

func (t *Tool) Schema() tool.Schema {
    return tool.Schema{
        Name:        "<name>",
        Description: "What this tool does",
        Parameters: tool.Parameters{
            Type:       "object",
            Properties: map[string]tool.Property{
                // Define parameters
            },
        },
    }
}

func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
    // Implementation
    return "result", nil
}
```

`tool.Base` provides no-op `Pre`, `Post`, and `Paths`. Override only what you need.

### 2. Opt-in interfaces

Implement these as needed (type-asserted at call sites, not declared on Base):

| Interface          | Method                                  | Default  | Purpose                               |
| ------------------ | --------------------------------------- | -------- | ------------------------------------- |
| `PreHook`          | `Pre(ctx, args) error`                  | no-op    | Validation before execution           |
| `PostHook`         | `Post(ctx, args)`                       | no-op    | Cleanup after execution               |
| `PathDeclarer`     | `Paths(ctx, args) (read, write, error)` | no paths | Filesystem paths for Landlock sandbox |
| `SandboxOverride`  | `Sandboxable() bool`                    | true     | Whether tool runs in sandbox          |
| `ParallelOverride` | `Parallel() bool`                       | true     | Whether tool can run in parallel      |
| `LSPAware`         | `WantsLSP() bool`                       | false    | Whether tool needs LSP manager        |
| `Previewer`        | `Preview(ctx, args) (string, error)`    | none     | Preview output before execution       |
| `Overrider`        | `Overrides() bool`                      | false    | Plugin tool overriding a built-in     |
| `Closer`           | `Close()`                               | none     | Cleanup on shutdown                   |

### 3. Register the tool

Add the tool in `internal/tools/tools.go` (`All()` function). Tool assembly (base tools + Task + Batch + ToolDefs) is handled by `internal/tools/assemble` (`assemble.Tools()`), so new always-present tools only need to be added in one place.

### 4. Consider both execution paths

Tools execute via two paths:

- **Direct**: in-process, full Go context available
- **Sandboxed**: child process re-exec with Landlock. No parent state survives — args arrive as CLI JSON, SDK context as stdin JSON, results return as stdout JSON

If your tool relies on parent-process state (context values, cached data), it will break under sandbox. Design for both paths.

### 5. Test

```bash
# Build
go build -o /tmp/aura-test/aura .

# Verify schema appears
/tmp/aura-test/aura tools <name>

# Verify execution (use --agent=high for tool access)
/tmp/aura-test/aura --agent=high --include-tools <Name> run "Use <name> to ..."
```

---

## Adding a New Provider

### 1. Create the provider package

Create `pkg/providers/<name>/` with at minimum:

- `client.go` — HTTP client setup, base URL, auth
- `chat.go` — `Chat()` implementation with streaming
- `models.go` — `Models()` and `Model()` listing

### 2. Implement core methods

Implement `Chat()`, `Models()`, `Model()`, `Estimate()`. Return `providers.ErrEstimateNotSupported` from `Estimate()` if native estimation is unavailable.

### 3. Implement optional capabilities

Opt-in interfaces for additional capabilities:

```go
// providers.Embedder — for embedding support
func (c *Client) Embed(ctx context.Context, req embedding.Request) (embedding.Response, usage.Usage, error)

// providers.Reranker — for reranking support
func (c *Client) Rerank(ctx context.Context, req rerank.Request) (rerank.Response, error)

// providers.Transcriber — for audio transcription
func (c *Client) Transcribe(ctx context.Context, req transcribe.Request) (transcribe.Response, error)

// providers.Synthesizer — for speech synthesis
func (c *Client) Synthesize(ctx context.Context, req synthesize.Request) (synthesize.Response, error)
```

Callers use `providers.As[providers.Embedder](provider)` to check capabilities at runtime.

### 4. Error handling

`errors.go` — `handleError()` calling `providers.ClassifyHTTPError()` to map HTTP responses to typed errors (rate limit, auth failure, context exceeded, etc.).

### 5. Register in factory

`internal/providers/factory.go` — add a case in the switch. The factory wraps instances with `RetryProvider`.

### 6. Add config and example

- `internal/config/providers.go` — add the config type
- `.aura/config/providers/` — add an example config file

### 7. Test

```bash
/tmp/aura-test/aura --provider=<name> --model=<model> run "hello"
/tmp/aura-test/aura --provider=<name> models
```

---

## Adding a New Hook Timing

Hook timings define when plugin code runs during the assistant lifecycle. Timings fall into two categories: **typed** (carry a timing-specific modification field) and **generic** (only produce base `Injection` values).

### 1. Add timing constant

`internal/injector/injector.go` — add to the `Timing` enum and `AllTimings` slice.

### 2. Add timing name

`internal/injector/registry.go` — add to `timingNames` (maps `Timing` → display string).

### 3. Define typed injection struct and checker interface (typed timings only)

`internal/injector/injector.go` — add if the timing carries a modification field (like `Request`, `Output`, `Compaction`):

```go
type MyTimingInjection struct {
    Injection
    MyField *sdk.MyModification
}

func (inj MyTimingInjection) Base() Injection { return inj.Injection }

type MyTimingChecker interface {
    CheckMyTiming(ctx context.Context, state *State) *MyTimingInjection
}
```

### 4. Add typed registry method (typed timings only)

`internal/injector/registry.go` — add a `RunMyTiming()` method using the `runCheckers` generic helper:

```go
func (r *Registry) RunMyTiming(ctx context.Context, state *State) []MyTimingInjection {
    return runCheckers(r, ctx, MyTiming, castTo[MyTimingChecker], MyTimingChecker.CheckMyTiming, state)
}
```

### 5. Add timing registry entry

`internal/plugins/dispatch.go` — add to `timingRegistry` with `assign` (stores function on `Hook`) and `dispatch` (calls it with the SDK context).

### 6. Add Hook struct field and CheckXxx method

`internal/plugins/hook.go` — add a function field. For typed timings: implement `CheckMyTiming()` via `prepareAndDispatch` + `buildBaseInjection` and add the timing to the guard in `Check()`. For generic timings: `Check()` handles it automatically.

### 7. Define SDK context type

`sdk/sdk.go` — add a context struct (e.g., `OnSessionEndContext`) with the fields plugins need.

### 8. Register symbols

`sdk/symbols.go` — add the new context type to the Yaegi symbol map.

### 9. Wire into assistant

In the appropriate assistant file (`loop.go`, `tools.go`, `compact.go`), call `RunMyTiming()` at the injection point. Use `injector.Bases()` when passing to `injectMessages`.

### 10. Test

Create a test plugin that exports a function matching the new timing signature, run with `--debug`, and verify the hook fires:

```bash
/tmp/aura-test/aura --debug --agent=high run "trigger the timing"
# Check .aura/debug.log for hook dispatch entries
```

---

## Adding a Config Entity Type

### 1. Define the type

Create `internal/config/<name>.go` with the struct definition. Implement the `Namer` interface:

```go
type MyEntity struct {
    name string  // populated from map key during loading
    // fields...
}

func (e MyEntity) Name() string { return e.name }
```

### 2. Choose the collection type

- **File-keyed** (most entities): use `Collection[T]` — one entity per file, keyed by `file.File`
- **String-keyed** (multi-per-file like providers, MCPs): use `StringCollection[T]` — keyed by entity name

### 3. Add to Config struct

In `internal/config/config.go`, add a field of type `Collection[MyEntity]` or `StringCollection[MyEntity]`.

### 4. Add Part constant

In `internal/config/part.go`, add `Part<Name>` to the constants.

### 5. Add file discovery

In `internal/config/files.go`, in the `Load()` method, add directory scanning for your entity's config path.

### 6. Add validation

In `internal/config/validate.go`, add a validation block for your entity. Use struct validation tags and any custom checks.

### 7. Add loader

Choose the parsing path based on format:

- **Markdown with frontmatter** (agents, modes, systems, skills, commands) → use `frontmatter.LoadRaw()` to extract YAML frontmatter and body
- **Pure YAML** (providers, features, hooks, tasks, mcp, lsp, sandbox, tools, approval-rules) → use `yamlutil.StrictUnmarshal()` with unknown-field rejection

The generic `Collection[T]` type provides `Get(name)`, `Names()`, `Filter()`, and other methods automatically.

---

## Adding a Slash Command

Three paths, from simplest to most powerful:

### Custom command (no code)

Create `.aura/config/commands/<name>.md` with YAML frontmatter:

```markdown
---
name: <name>
description: What this command does
hints:
  - usage hint
---

Prompt template body. Supports {{ .Agent }}, {{ .Mode.Name }}, {{ .Args }}.
```

### Built-in command

Create a file in `internal/slash/commands/` and register the command via `All()`. Set the `Category` field to group the command in `/help` output (e.g. `"agent"`, `"session"`, `"tools"`, `"context"`, `"execution"`, `"config"`, `"system"`). Built-in commands have access to the full `slash.Context` (assistant state, config, builder).

### Plugin command

Export a function matching the command signature in a Go plugin. The SDK provides `sdk.CommandSchema` and `sdk.CommandResult` types.

---

## Writing a Plugin

Plugins are Go source loaded at runtime via Yaegi. Three types:

### Hook plugin

Export functions matching timing signatures (e.g., `BeforeToolExecution(sdk.BeforeToolContext) sdk.Result`). The plugin loader probes for exported functions and auto-discovers hooks.

### Tool plugin

Export `ToolSchema() sdk.ToolSchema` and `ToolExecute(sdk.Context, map[string]any) (string, error)`. Set `override: true` in the schema to replace a built-in tool.

### Command plugin

Export `CommandSchema() sdk.CommandSchema` and `CommandExecute(sdk.Context, string) sdk.CommandResult`.

### Plugin structure

Every plugin needs:

```
my-plugin/
  main.go       # Go source with exported functions
  plugin.yaml   # Metadata (description, author, version)
  go.mod        # Module definition (imports sdk)
  go.sum        # Dependency checksums
```

The package name must match the directory name (with hyphens converted to underscores). `plugin.yaml` is metadata-only — hooks are auto-discovered.

### SDK compatibility

The host checks plugin SDK version at load time. Plugins must use a compatible SDK version. See `docs/features/plugins.md` for version compatibility details.

{% endraw %}
