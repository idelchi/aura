---
layout: default
title: Prompts
parent: Configuration
nav_order: 6
---

{% raw %}

# System Prompts

System prompts define the agent's personality, behavior, and constraints. Each prompt is a Markdown file with YAML frontmatter in `.aura/config/prompts/`. The loader discovers all `**/*.md` files recursively — organize by purpose (e.g. `system/`, `features/`).

## Frontmatter Schema

```yaml
---
name: MyPrompt # Unique identifier. Referenced by agent `system:` field.
description: Purpose.
inherit: [Base] # Parent prompts — bodies concatenated in order.
---
```

## Template Variables

The body is rendered as a Go template before being sent to the LLM. Sprig functions are available.

| Variable                     | Description                                   |
| ---------------------------- | --------------------------------------------- |
| `{{ .Model.Name }}`          | Resolved model name                           |
| `{{ .Model.Family }}`        | Model family string                           |
| `{{ .Model.ParameterSize }}` | Human parameter string (e.g. `"8B"`)          |
| `{{ .Model.ContextLength }}` | Context window length in tokens               |
| `{{ .Model.Thinking }}`      | Extended thinking supported (bool)            |
| `{{ .Model.Vision }}`        | Vision supported (bool)                       |
| `{{ .Model.Tools }}`         | Tool use supported (bool)                     |
| `{{ .Provider }}`            | Active provider name                          |
| `{{ .Agent }}`               | Active agent name                             |
| `{{ .Mode.Name }}`           | Active mode name                              |
| `{{ .Tools.Eager }}`         | Resolved eager tool names (range-iterable)    |
| `{{ .Tools.Deferred }}`      | XML block listing deferred tools              |
| `{{ .Files }}`               | Autoloaded file entries from agent `files:`   |
| `{{ .Workspace }}`           | Injected AGENTS.md workspace instructions     |
| `{{ .Sandbox.Enabled }}`     | Sandbox enforcing (bool)                      |
| `{{ .Sandbox.Display }}`     | Pre-rendered restriction text                 |
| `{{ .ReadBefore.Write }}`    | Read-before-write enforced (bool)             |
| `{{ .ReadBefore.Delete }}`   | Read-before-delete enforced (bool)            |
| `{{ .ToolPolicy.Display }}`  | Pre-rendered tool policy text                 |
| `{{ .Hooks.Active }}`        | Active hooks (range-iterable with `.Name`, `.Description`, `.Event`, `.Matcher`, `.Files`, `.Command`) |
| `{{ .Hooks.Display }}`       | Pre-rendered hook summary text                |
| `{{ .Memories.Local }}`      | Local memory entries (`.aura/memory/*.md`)    |
| `{{ .Memories.Global }}`     | Global memory entries (`~/.aura/memory/*.md`) |
| `{{ .Config.Global }}`       | Global config home (`~/.aura`)                |
| `{{ .Config.Project }}`      | Project config path                           |
| `{{ .Config.Source }}`       | Agent's source config home                    |
| `{{ .LaunchDir }}`           | CWD at process start                          |
| `{{ .WorkDir }}`             | Working directory after `--workdir`           |
| `{{ env "VAR" }}`            | Environment variable                          |
| `{{ index .Vars "key" }}`    | Template variable from `--set`                |

Example:

```
Date: {{ now | date "2006-01-02" }}

{{ if .Tools.Eager -}}
Tools:
{{ range .Tools.Eager }}- {{ . }}
{{ end }}
{{ end -}}
```

## Inheritance

Child body is appended after parent bodies. Duplicate parents are resolved once.

```yaml
# prompts/system/base.md
---
name: Base
---
You are Aura, a coding assistant.
```

```yaml
# prompts/system/agentic.md
---
name: Agentic
inherit: [Base]
---
## Critical Rules
1. Complete every task before stopping.
```

Result: Base body + `"\n\n"` + Agentic body.

Multi-parent: `inherit: [A, B]` produces A body + B body + self body.

Note: agents and modes use **replace** semantics (child body replaces parent if non-empty). Prompts always **concatenate**.

## Composition

System prompts act as compositors that control the order of all prompt components. Use these directives at the end of your system prompt:

```
{{ template "agent" . }}

{{ range .Files }}
### {{ .Name }}
{{ include .TemplateName $ }}
{{ end }}

{{ range .Workspace }}
## {{ .Type }}
{{ include .TemplateName $ }}
{{ end }}

{{ if .Mode.Name }}
## Active Mode: {{ .Mode.Name }}
{{ template "mode" . }}
{{ end }}
```

You can reorder or omit components. When an agent has `system: ""`, a default compositor is used that includes agent, files, workspace, and mode in the standard order.

## Shipped Prompts

| Name         | Directory   | Purpose                                         |
| ------------ | ----------- | ----------------------------------------------- |
| `Agentic`    | `system/`   | Full agentic coding prompt                      |
| `Chat`       | `system/`   | Conversational — discourages proactive tool use |
| `Lite`       | `system/`   | Stripped-down agentic prompt for smaller models |
| `Compaction` | `features/` | Summarizes conversation for context handoff     |
| `Thinking`   | `features/` | Thinking block summarization                    |

Feature prompts are used internally by the compaction and thinking systems via the `prompt:` field in feature config.

{% endraw %}
