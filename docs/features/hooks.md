---
layout: default
title: Hooks
parent: Features
nav_order: 6
---

# Hooks

Hooks are user-configurable shell commands that run automatically before or after LLM tool execution, enabling formatting, linting, and validation without LLM involvement. Defined in `.aura/config/hooks/*.yaml` as a YAML map of hook names to definitions.

## Hook Fields

| Field         | Type     | Default | Description                                                                                           |
| ------------- | -------- | ------- | ----------------------------------------------------------------------------------------------------- |
| `description` | string   |         | Summary shown in system prompts via `.Hooks.Active` template data                                     |
| `event`       | string   |         | `"pre"` (before Execute, can block) or `"post"` (after Execute)                                       |
| `matcher`     | string   |         | Regex matched against tool name. Empty = match all tools                                              |
| `files`       | string   |         | Glob matched against file basenames. Hook skipped if no match                                         |
| `command`     | string   |         | Shell command executed via mvdan/sh. Receives JSON on stdin                                           |
| `timeout`     | int      | `10`    | Seconds before the hook is killed                                                                     |
| `depends`     | []string | `[]`    | Hook names (same event) that must run before this hook                                                |
| `inherit`     | []string | `[]`    | Hook names to inherit fields from — explicitly set fields override                                    |
| `silent`      | bool     | `false` | Discard all output and exit codes                                                                     |
| `disabled`    | \*bool   | `false` | Skip this hook. Pointer semantics: absent = inherit from parent, `false` = enabled, `true` = disabled |

## JSON Stdin

Every hook receives a JSON object on stdin. `hook_event` is `"PreToolUse"` or `"PostToolUse"`. `tool.output` is only present in post hooks. `file_paths` is extracted from tool input and may be empty.

```json
{
  "hook_event": "PreToolUse",
  "tool": {
    "name": "Write",
    "input": { "path": "main.go", "content": "..." },
    "output": "..."
  },
  "cwd": "/home/user/project",
  "file_paths": ["/home/user/project/main.go"]
}
```

## $FILE and Exit Codes

When file paths are present, `$FILE` is set to a space-separated list of matched paths. Use it in commands: `gofmt -w $FILE`.

## Exit Code Semantics

| Exit code | Pre hook                  | Post hook                 |
| --------- | ------------------------- | ------------------------- |
| `0`       | Parse stdout for JSON     | Parse stdout for JSON     |
| `2`       | Block tool execution      | Append stderr as feedback |
| other     | Append stderr as feedback | Append stderr as feedback |

## JSON Stdout (exit 0)

On exit 0, a hook may optionally print JSON to stdout:

```json
{
  "message": "Formatted successfully.",
  "deny": true,
  "reason": "File contains forbidden pattern."
}
```

| Field     | Type   | Description                                         |
| --------- | ------ | --------------------------------------------------- |
| `message` | string | Text appended to tool output as feedback to the LLM |
| `deny`    | bool   | `true` to block the tool (pre only)                 |
| `reason`  | string | Reason shown when `deny` is true                    |

Non-JSON stdout is treated as a plain-text message.

## DAG Ordering via `depends`

Hooks within the same event run in topological order; unrelated hooks run alphabetically. Cycles and missing dependencies are hard errors at config load.

```yaml
go:lint:
  depends: [go:fix] # runs after go:fix completes
```

## Config Inheritance via `inherit`

```yaml
go:fix:
  inherit: [go:format] # copies event, matcher, files, timeout, silent from go:format
  depends: [go:format]
  command: golangci-lint run --fix $FILE
```

## Per-Agent and Per-Mode Filtering

Use `hooks.enabled` / `hooks.disabled` in agent or mode frontmatter:

```yaml
---
name: MyAgent
hooks:
  disabled: ["go:*"] # disable all Go hooks for this agent
---
```

Patterns support `*` wildcards. Filter chain: **agent → mode**. Excluding a hook also prunes any hooks that depend on it. Use `/hooks` to see the filtered view for the current agent and mode.

## Prompt Awareness

Active hooks are available as template data in system, agent, and mode prompts. The `description` field is what appears — hooks without one show their name and event only.

`.Hooks.Display` gives a pre-rendered summary; `.Hooks.Active` is a slice with fields `Name`, `Description`, `Event`, `Matcher`, `Files`, and `Command`.

```
{%- raw %}
{{- if .Hooks.Active }}
Hooks that run after your tool calls:
{{ range .Hooks.Active -}}
- {{ .Name }}: {{ .Description }} ({{ .Event }}, files: {{ .Files }})
{{ end -}}
{{- end }}
{% endraw %}
```

## Example: Go Hook Chain

```yaml
# .aura/config/hooks/go.yaml

go:format:
  event: post
  matcher: "Patch|Write"
  files: "*.go"
  silent: true
  command: golangci-lint fmt $FILE
  timeout: 15

go:fix:
  inherit: [go:format]
  depends: [go:format]
  command: |
    golangci-lint run --fix $FILE
    for f in $FILE; do
      while gopls codeaction -kind=quickfix -exec -write $f 2>/dev/null; do :; done
      gopls codeaction -kind=source.organizeImports -exec -write $f 2>/dev/null
    done
  timeout: 30

go:lint:
  inherit: [go:format]
  depends: [go:fix]
  silent: false
  command: golangci-lint run $FILE
```

Execution order: `go:format` → `go:fix` → `go:lint`.

## Example: Pre-Hook Validation

```yaml
# .aura/config/hooks/validate.yaml

validate-bash:
  event: pre
  matcher: "Bash"
  command: .aura/hooks/validate-bash.sh
  timeout: 5
```

If `.aura/hooks/validate-bash.sh` exits with code 2, the Bash tool call is blocked and the LLM receives the script's stderr as the rejection reason.

## Message Injection (Injectors)

Injectors are plugins that fire at specific points in the assistant loop and inject messages into the conversation to guide LLM behavior. Unlike shell hooks (which run commands), injectors are Go plugins that produce structured feedback fed directly to the LLM.

### Shipped Injectors

| Plugin                    | Timing             | Purpose                                                                                 |
| ------------------------- | ------------------ | --------------------------------------------------------------------------------------- |
| `todo-reminder`           | BeforeChat         | Reminds about pending todos every N iterations                                          |
| `max-steps`               | BeforeChat         | Disables tools when iteration limit reached                                             |
| `session-stats`           | BeforeChat         | Shows session stats summary (turns, tool calls, context usage, top tools) every 5 turns |
| `todo-not-finished`       | AfterResponse      | Warns if todos incomplete and no tool calls                                             |
| `empty-response`          | AfterResponse      | Handles empty LLM responses                                                             |
| `done-reminder`           | AfterResponse      | Reminds LLM to call Done tool when stopping                                             |
| `loop-detection`          | AfterToolExecution | Detects repeated identical tool calls                                                   |
| `failure-circuit-breaker` | AfterToolExecution | Stops after 3+ consecutive different tool failures                                      |
| `repeated-patch`          | AfterToolExecution | Warns after 5+ patches to same file                                                     |

Example injectors ship under `.aura/plugins/injectors/` and are not installed by `aura init`. Copy them to your own `.aura/plugins/` directory to activate them.

### Conditions

Injectors use a `condition` expression in `plugin.yaml` evaluated before every hook call. The condition syntax is identical to `/assert` — see [/assert](#assert--conditional-actions). Set `once: true` to fire only on the first match.

### Custom Injectors

To write a custom injector, add a Go plugin under `.aura/plugins/` implementing one of the nine hook timing points (`BeforeChat`, `AfterResponse`, `BeforeToolExecution`, `AfterToolExecution`, `OnError`, `BeforeCompaction`, `AfterCompaction`, `OnAgentSwitch`, `TransformMessages`). See [Plugins]({{ site.baseurl }}/features/plugins) for the full authoring guide.
