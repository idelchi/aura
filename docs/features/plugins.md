---
layout: default
title: Plugins
parent: Features
nav_order: 13
---

# Plugins

Plugins are Go modules that hook into the conversation lifecycle. They run real Go code — track state, modify tool arguments, inject messages, import third-party libraries. Plugins live under `.aura/plugins/` and are discovered recursively at startup.

## Choosing an Extensibility Mechanism

| I want to...                                                           | Use               | Where                          |
| ---------------------------------------------------------------------- | ----------------- | ------------------------------ |
| Add a command the user types (`/foo`)                                  | Custom command    | `.aura/config/commands/foo.md` |
| Add a capability the LLM invokes autonomously                          | Skill             | `.aura/skills/foo.md`          |
| Override a tool's description, usage, or examples                      | Tool definition   | `.aura/config/tools/foo.yaml`  |
| Run a shell script before/after tool execution                         | Hook              | `.aura/config/hooks/foo.yaml`  |
| React to events, modify tool args/output, inject messages, track state | Plugin (injector) | `.aura/plugins/foo/`           |
| Define a new tool the LLM can call                                     | Plugin (tool)     | `.aura/plugins/foo/`           |
| Connect an external tool server                                        | MCP server        | `.aura/config/mcp/foo.yaml`    |

Custom commands and skills are Markdown files — zero Go required. Hooks are YAML + shell. Plugins are Go modules for when you need state, logic, or custom tools.

See: [Custom Commands]({{ site.baseurl }}/features/slash-commands#custom-slash-commands), [Skills]({{ site.baseurl }}/features/tools#skills), [Tool Definitions]({{ site.baseurl }}/features/tools#tool-definitions), [Hooks]({{ site.baseurl }}/features/hooks), [MCP]({{ site.baseurl }}/features/mcp).

## Plugin Packs

A single git repository can contain multiple plugins. Each subdirectory with a `plugin.yaml` is an independent plugin — the repository is cloned once and all contained plugins are loaded.

```
aura plugins add https://github.com/user/aura-plugins-collection
```

## Directory Structure

```
.aura/plugins/
├── my-plugin/
│   ├── go.mod
│   ├── plugin.yaml
│   ├── main.go
│   └── vendor/           # optional: vendored third-party deps
└── tools/
    ├── gotify/
    │   ├── go.mod
    │   ├── plugin.yaml
    │   └── main.go
    └── notepad/
        ├── go.mod
        ├── plugin.yaml
        └── main.go
```

Plugin identity defaults to the leaf directory name, overridable with `name:` in `plugin.yaml`. Subdirectory nesting is invisible to the system. All `.go` files in a plugin directory must declare the same package name, derived from the module path in `go.mod`.

## Plugin Config

```yaml
# name: my-custom-name  # override plugin identity (default: leaf directory name)
description: "What this plugin does"
disabled: false
override: false # replace a built-in tool with the same name
opt_in: false # tools only — hidden unless explicitly enabled by name
condition: "auto" # injectors only: condition expression
once: false # injectors only: fire only once per session
env: # optional: env var names the plugin may read
  - MY_API_KEY
# config:       # default config values (accessible via sdk.Context.PluginConfig)
#   max_failures: 3
```

| Field         | Default | Description                                                                                                                     |
| ------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `name`        | --      | Override plugin identity (default: leaf directory name)                                                                         |
| `description` | --      | Human-readable description                                                                                                      |
| `disabled`    | `false` | Skip loading                                                                                                                    |
| `override`    | `false` | Replace an existing tool with the same name. Without this, name conflicts are a startup error.                                  |
| `opt_in`      | `false` | Tools only — hidden unless explicitly enabled by name at any filter layer                                                       |
| `condition`   | --      | Injectors only — evaluated before every hook call (see [Conditions](#conditions))                                               |
| `once`        | `false` | Injectors only — fire only once per session when condition first becomes true                                                   |
| `env`         | `[]`    | Environment variable names the plugin may read                                                                                  |
| `config`      | `{}`    | Default config values, accessible via `sdk.Context.PluginConfig`. Overridden by `features/plugins.yaml` global and local config |

## Creating a Plugin

```sh
mkdir -p .aura/plugins/my-plugin
cd .aura/plugins/my-plugin
go mod init my-plugin
go get github.com/idelchi/aura/sdk@latest
```

The module name determines your Go package name. `my-plugin` becomes `package my_plugin`. For third-party dependencies: `go get github.com/some/library && go mod vendor`. Create `plugin.yaml`:

```yaml
description: "My custom plugin"
```

## Hook Functions

All hooks are optional. Hooks only fire in `aura run`. Plugin tools are available in both `aura run` and `aura tools`.

| Hook                  | Signature                                                                    | When it fires                                               | Returns                                                                                                                                                                                          |
| --------------------- | ---------------------------------------------------------------------------- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `BeforeChat`          | `func(context.Context, sdk.BeforeChatContext) (sdk.Result, error)`           | Before sending messages to the LLM                          | Inject message, append system prompt (`RequestModification`), or skip the LLM call                                                                                                               |
| `AfterResponse`       | `func(context.Context, sdk.AfterResponseContext) (sdk.Result, error)`        | After receiving the LLM response                            | Skip storing to history, or replace content (`ResponseModification`)                                                                                                                             |
| `BeforeToolExecution` | `func(context.Context, sdk.BeforeToolContext) (sdk.BeforeToolResult, error)` | Before a tool call executes                                 | Modified args (re-validated against schema), or `Block: true` to prevent execution                                                                                                               |
| `AfterToolExecution`  | `func(context.Context, sdk.AfterToolContext) (sdk.Result, error)`            | After a tool call completes                                 | Rewrite tool output sent to the LLM via `Result.Output`                                                                                                                                          |
| `OnError`             | `func(context.Context, sdk.OnErrorContext) (sdk.Result, error)`              | After all built-in recovery has failed                      | `Retry: true` (max 3×) or `Skip: true` to suppress; error types: `rate_limit`, `auth`, `network`, `server`, `content_filter`, `credit_exhausted`, `model_unavailable`, `context_exhausted`, `""` |
| `BeforeCompaction`    | `func(context.Context, sdk.BeforeCompactionContext) (sdk.Result, error)`     | Before context compaction begins                            | `CompactionModification{Skip: true}` to bypass built-in compaction                                                                                                                               |
| `AfterCompaction`     | `func(context.Context, sdk.AfterCompactionContext) (sdk.Result, error)`      | After context compaction completes                          | Observe only (`Success`, `PreMessages`, `PostMessages`, `SummaryLength`)                                                                                                                         |
| `OnAgentSwitch`       | `func(context.Context, sdk.OnAgentSwitchContext) (sdk.Result, error)`        | When the active agent changes                               | Observe only (`PreviousAgent`, `NewAgent`, `Reason`)                                                                                                                                             |
| `TransformMessages`   | `func(context.Context, sdk.TransformContext) ([]sdk.Message, error)`         | Inside `chat()` after filtering, before the request is sent | Modified message array for this call only — history unaffected; plugins chain alphabetically; on error, original array is used                                                                   |

Example — inject a status line every 5 turns:

```go
func BeforeChat(_ context.Context, hctx sdk.BeforeChatContext) (sdk.Result, error) {
    callCount++
    if callCount%5 != 0 {
        return sdk.Result{}, nil
    }
    return sdk.Result{
        Message: fmt.Sprintf("Turn %d | tokens: %d (%.0f%%)", callCount, hctx.Tokens.Estimate, hctx.Tokens.Percent),
        Role:    sdk.RoleUser,
        Prefix:  "[my-plugin] ",
        Eject:   true,
    }, nil
}
```

## Command Functions

```go
func Command() sdk.CommandSchema {
    return sdk.CommandSchema{
        Name:        "greet",
        Description: "Say hello to someone.",
        Hints:       []string{"name"},
    }
}

func ExecuteCommand(_ context.Context, args string, _ sdk.Context) (sdk.CommandResult, error) {
    if args == "" {
        args = "world"
    }
    return sdk.CommandResult{Output: fmt.Sprintf("Hello, %s!", args)}, nil
}
```

`CommandSchema.Forward: true` sends output to the LLM as a user message. Built-in commands take priority over custom commands, which take priority over plugin commands.

## Tool Functions

```go
func Schema() sdk.ToolSchema { ... }                                              // required
func Execute(ctx context.Context, sc sdk.Context, args map[string]any) (string, error) { ... } // required
func Paths(args map[string]any) (sdk.ToolPaths, error) { ... }                   // optional
func Sandboxable() bool { ... }                                                   // optional, default true
func Parallel() bool { ... }                                                      // optional, default true
func Init(cfg sdk.ToolConfig) { ... }                                             // optional
func Available() bool { ... }                                                     // optional
```

`ToolPaths` has five path lists: `Read`, `Write` (sandbox permissions), `Record` (mark file as seen by LLM), `Guard` (require prior read), `Clear` (invalidate prior read).

The `notepad` example plugin under `.aura/plugins/tools/notepad/` has a complete `Schema()`, `Execute()`, and `Paths()` implementation.

```sh
aura tools Notepad
aura tools Notepad action=read path=/tmp/file.txt
aura tools Notepad '{"action": "write", "path": "/tmp/file.txt", "content": "hello"}'
```

## SDK

Every hook receives a base `sdk.Context` with runtime state (agent, mode, tokens, model info, session stats, tool history) plus timing-specific fields:

| Context                   | Extra Fields                                                                                  |
| ------------------------- | --------------------------------------------------------------------------------------------- |
| `BeforeChatContext`       | _(base context only)_                                                                         |
| `AfterResponseContext`    | `Response string`, `Thinking string`, `Calls []ToolCall`                                      |
| `BeforeToolContext`       | `ToolName string`, `Arguments map[string]any`                                                 |
| `AfterToolContext`        | `ToolName`, `ToolResult`, `ToolError`, `ToolDuration`                                         |
| `OnErrorContext`          | `Error string`, `ErrorType string`, `Retryable bool`, `StatusCode int`                        |
| `BeforeCompactionContext` | `Forced bool`, `TokensUsed int`, `ContextPercent float64`, `MessageCount int`, `KeepLast int` |
| `AfterCompactionContext`  | `Success bool`, `PreMessages int`, `PostMessages int`, `SummaryLength int`                    |
| `OnAgentSwitchContext`    | `PreviousAgent string`, `NewAgent string`, `Reason string`                                    |
| `TransformContext`        | `Messages []Message`                                                                          |

Return `sdk.Result{}` (empty) to skip injection. Plugin errors are non-fatal — they log and skip.

## Imports and Unsafe Mode

| Category        | Packages                                                                   |
| --------------- | -------------------------------------------------------------------------- |
| **SDK**         | `github.com/idelchi/aura/sdk`                                              |
| **Stdlib**      | All standard library packages (`fmt`, `strings`, `net/http`, `time`, etc.) |
| **Third-party** | Any package vendored via `go mod vendor`                                   |
| **Restricted**  | `os/exec`, `syscall` — require unsafe mode (see below)                     |

`os/exec` is blocked by default. Enable it:

```yaml
# .aura/config/features/plugins.yaml
plugins:
  unsafe: true
```

Or: `aura --unsafe-plugins`

## Environment Variables

`os.Getenv` returns empty strings by default. Grant access in `plugin.yaml`:

```yaml
env:
  - MY_API_KEY
  - MY_API_URL
```

Use `"*"` to grant access to all environment variables.

## Plugin Feature Config

```yaml
# .aura/config/features/plugins.yaml
plugins:
  dir: "" # plugin directory (relative to home; empty = "plugins/")
  unsafe: false # allow os/exec in plugins
  include: [] # only load these plugins (empty = all, supports wildcards)
  exclude: [] # skip these plugins (applied after include, supports wildcards)
```

### Plugin Config Values

Three layers (lowest to highest precedence):

1. Plugin defaults — `config:` in `plugin.yaml`
2. Global — `plugins.config.global` (sent to all plugins)
3. Local — `plugins.config.local.<plugin-name>` (per-plugin overrides)

```yaml
plugins:
  config:
    global:
      verbose: true
    local:
      failure-circuit-breaker:
        max_failures: 5
```

Access via `ctx.PluginConfig["key"].(int)` — YAML integers decode as Go `int`, not `float64`.

## Limitations

- No generics — `slices.SortFunc`, `maps.Keys`, and similar generic stdlib functions are unsupported.
- No `min`/`max` builtins — use `math.Min`/`math.Max`.
- No cgo or assembly — plugins and vendored deps must be pure Go.
- No range-over-func (Go 1.23 iterators).
- Goroutines work but hooks must return synchronously; goroutines are fire-and-forget.
- Panics are caught and surfaced as display-only errors.

## Example Plugins

The repository includes examples under `.aura/plugins/` (not installed by `aura init`):

**Injectors:**

| Plugin                    | Timing             | What                                               |
| ------------------------- | ------------------ | -------------------------------------------------- |
| `todo-reminder`           | BeforeChat         | Reminds about pending todos every N iterations     |
| `max-steps`               | BeforeChat         | Disables tools when iteration limit reached        |
| `session-stats`           | BeforeChat         | Shows session stats every 5 turns                  |
| `todo-not-finished`       | AfterResponse      | Warns if todos incomplete and no tool calls        |
| `empty-response`          | AfterResponse      | Handles empty LLM responses                        |
| `done-reminder`           | AfterResponse      | Reminds LLM to call Done when stopping             |
| `loop-detection`          | AfterToolExecution | Detects repeated identical tool calls              |
| `failure-circuit-breaker` | AfterToolExecution | Stops after 3+ consecutive different tool failures |
| `repeated-patch`          | AfterToolExecution | Warns after N patches to same file                 |

**Diagnostics:**

| Plugin                    | Timing             | What                                               |
| ------------------------- | ------------------ | -------------------------------------------------- |
| `tool-logger`             | AfterToolExecution | Logs every tool call and result to a file          |
| `turn-tracker`            | BeforeChat         | Injects a status line every N turns                |

**Tools:**

| Plugin    | What                                         |
| --------- | -------------------------------------------- |
| `gotify`  | Sends push notifications via Gotify (opt-in) |
| `notepad` | Read/write scratch files                     |

## Conditions

The `condition` field in `plugin.yaml` accepts an expression. Invalid expressions are rejected at config load time.

### Boolean Conditions

| Condition      | True when                          |
| -------------- | ---------------------------------- |
| `todo_empty`   | No todo items exist                |
| `todo_done`    | All todo items completed           |
| `todo_pending` | Pending or in-progress items exist |
| `auto`         | Auto mode is enabled               |

### Comparison Conditions

| Greater-than           | Less-than              | Compares               |
| ---------------------- | ---------------------- | ---------------------- |
| `context_above:<N>`    | `context_below:<N>`    | Token usage percentage |
| `history_gt:<N>`       | `history_lt:<N>`       | Message count          |
| `tool_errors_gt:<N>`   | `tool_errors_lt:<N>`   | Tool error count       |
| `tool_calls_gt:<N>`    | `tool_calls_lt:<N>`    | Tool call count        |
| `turns_gt:<N>`         | `turns_lt:<N>`         | LLM turn count         |
| `compactions_gt:<N>`   | `compactions_lt:<N>`   | Compaction count       |
| `iteration_gt:<N>`     | `iteration_lt:<N>`     | Current iteration      |
| `tokens_total_gt:<N>`  | `tokens_total_lt:<N>`  | Cumulative tokens      |
| `model_context_gt:<N>` | `model_context_lt:<N>` | Model max context      |

### Model Conditions

| Condition                   | True when                                  |
| --------------------------- | ------------------------------------------ |
| `model_has:vision`          | Model supports vision/image input          |
| `model_has:tools`           | Model supports tool calling                |
| `model_has:thinking`        | Model supports reasoning output            |
| `model_has:thinking_levels` | Model supports configurable thinking depth |
| `model_is:<name>`           | Model name matches (case-insensitive)      |
| `model_params_gt:<N>`       | Parameter count > N billion                |
| `model_params_lt:<N>`       | Parameter count < N billion                |

### Filesystem & Shell Conditions

| Condition       | True when                                              |
| --------------- | ------------------------------------------------------ |
| `exists:<path>` | File or directory exists (relative to CWD or absolute) |
| `bash:<cmd>`    | Shell command exits 0 (120s timeout)                   |

### Negation and Composition

```yaml
condition: "not auto"
condition: "auto and context_above:70"
condition: "todo_pending and not turns_gt:50"
```

## Managing Plugins

```sh
# List and inspect
aura plugins list
aura plugins show <name>

# Install from git
aura plugins add https://github.com/user/aura-plugin-metrics
aura plugins add https://github.com/user/aura-plugin-metrics --ref v0.2.0
aura plugins add https://github.com/user/aura-plugins-collection --subpath tools/metrics
aura plugins add https://github.com/user/aura-plugin-metrics --global

# Install from local path
aura plugins add ./path/to/my-plugin --name my-plugin

# Update and remove
aura plugins update gotify
aura plugins update --all
aura plugins remove my-plugin
aura plugins remove tools         # remove entire pack
```

Authentication: public repos (no auth), then `GITLAB_TOKEN`/`GITHUB_TOKEN`/`GIT_TOKEN`, then `git credential fill`. SSH uses the SSH agent.

`aura plugins add` and `aura plugins update` run `go mod tidy && go mod vendor` automatically (requires Go in `PATH`). Pass `--no-vendor` to skip.

Pack plugins cannot be removed individually — set `disabled: true` in `plugin.yaml` to skip one.

## Lifecycle

| Event     | What happens                                                         |
| --------- | -------------------------------------------------------------------- |
| Startup   | Source loaded, interpreter created, hooks + tools probed, registered |
| Each turn | Hooks fire at their timing point; condition evaluated first if set   |
| `/reload` | Old interpreter destroyed, new one created from current files        |
| Shutdown  | Interpreter released                                                 |

Package-level variables persist across hook invocations but reset on `/reload`.

## Visibility

`/plugins` lists all loaded plugin hooks grouped by timing:

```
BeforeChat:
  todo-reminder/BeforeChat       enabled
  max-steps/BeforeChat           enabled

AfterToolExecution:
  loop-detection/AfterToolExecution    enabled
  my-plugin/BeforeChat                 enabled  condition="auto and context_above:70" once=true (fired)
```
