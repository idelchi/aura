---
layout: default
title: Modes
parent: Configuration
nav_order: 2
---

# Modes

A mode controls which tools are available, which shell commands the Bash tool can execute, and injects a behavioral system prompt. Mode files are Markdown with YAML frontmatter in `.aura/config/modes/`.

Use modes to create distinct operational contexts — for example, a read-only mode for exploration, a full-access mode for editing, or a planning mode that restricts destructive tools.

## Modes vs Agents

An **agent** is a full session configuration: model, provider, system prompt, tool set, fallback chain, included files. Switching agents changes _who_ is working.

A **mode** is an operational context _within_ an agent: tool filtering, hook filtering, execution policy, and feature tweaks. Switching modes changes _how_ you work — the model and prompt stay the same.

Use agents to switch models, providers, or personas. Use modes for same-model behavior variants — read-only exploration vs full editing, planning vs execution, restricted vs unrestricted tool access.

Override precedence: **global → agent → mode → task**. Each layer merges on top of the previous one.

## Frontmatter Schema

```yaml
---
name: ModeName # Unique identifier (shown in /mode and status bar)
inherit:
  [Ask] # Inherit from parent mode(s).
  # Key absent = inherit. Key present = replace. No append.
hide: false # Exclude from cycling (Tab) and listing (/mode), still chooseable via /mode <name>

tools:
  enabled: ["*"] # Glob patterns to enable
  disabled: [] # Glob patterns to disable (takes precedence)
  policy: # Tool execution policy (merged with agent-level policy)
    auto: [] # Tool/Bash patterns to auto-approve (run without asking)
    confirm: [] # Tool/Bash patterns requiring user approval before execution
    deny: # Tool/Bash patterns to hard-block (even in auto mode)
      - "Bash:rm *"
        # Pattern syntax: "ToolName", "Bash:command*", or "Tool:/path/*"

hooks:
  enabled: [] # Hook name patterns to include ([] = all). Supports * wildcards.
  disabled: [] # Hook name patterns to exclude. Cascade-prunes dependents.

description: | # Shown in /mode listing
  Human-readable description.

features: # Per-mode feature overrides (merged on top of agent features)
  sandbox:
    extra: # Additional sandbox paths (only effective when sandbox enabled)
      rw:
        - .
---
```

The Markdown body below the frontmatter is the mode's system prompt template. It supports Go template syntax:

{% raw %}

```
{{ if .Tools.Eager -}}
You have access to the following tools:
{{ range .Tools.Eager }}- {{ . }}
{{ end }}
{{ end -}}
```

{% endraw %}

## Tool Filtering

Tool filters form a chain: global filter (`tools.yaml` `enabled`/`disabled`, or CLI `--include-tools`/`--exclude-tools` which override config) applies first, then the agent's `tools.enabled`/`tools.disabled` patterns, then the mode's patterns further restrict access. Patterns support wildcards (e.g., `mcp__*` matches all MCP tools, `Todo*` matches all todo tools).

## Hook Filtering

Hook filters work the same way as tool filters: the agent's `hooks.enabled`/`hooks.disabled` patterns apply first, then the mode's patterns further restrict. When a hook is excluded and other hooks depend on it, dependents are cascade-pruned automatically (no DAG errors).

## Tool Policy Merging

Agent and mode policies are merged additively — patterns from both combine into a single effective policy. Within the merged policy, precedence is: `deny` > `confirm` > `auto` > default (auto).

Patterns support three forms:

- **Tool name**: `"Write"`, `"mcp__*"` — matches by tool name (glob)
- **Bash argument**: `"Bash:git push*"` — matches Bash tool calls where the command matches the suffix glob
- **Path argument**: `"Write:/tmp/*"`, `"Read:/etc/*"` — matches file-operating tools where the path matches the suffix glob

Persistent approval rules (saved when a user approves a `confirm` prompt) merge into the `auto` tier. The effective policy is injected into the system prompt so the LLM knows which tools require confirmation.

## Switching Modes

- **Keybinding**: `Tab` cycles through modes
- **Slash command**: `/mode [name]` switches to a specific mode or lists all
- **CLI flag**: `--mode <name>` sets the starting mode

## Example

```yaml
---
name: Ask
tools:
  enabled: ["*"]
  disabled: ["Mkdir", "Patch", "TodoCreate", "TodoProgress"]
  policy:
    deny:
      [
        "Bash:rm *",
        "Bash:mv *",
        "Bash:sudo *",
        "Bash:git push*",
        "Write:/etc/*",
      ]
description: |
  Read-only information gathering.
---
You are in read-only mode. Do not attempt to create or modify files.
```

## Feature Overrides

The `features:` block lets a mode override feature defaults. Non-zero values replace the agent's effective value for that field. Zero values and omitted fields inherit.

Override precedence: **global features → CLI flags → agent frontmatter → mode frontmatter → task definition**

Available feature keys match the top-level keys in `features/*.yaml`: `compaction`, `title`, `thinking`, `vision`, `embeddings`, `tools`, `stt`, `tts`, `sandbox`, `subagent`, `plugins`, `mcp`, `estimation`, `guardrail`. See [Features]({{ site.baseurl }}/configuration/features) for all available fields.

Common use case — extending sandbox paths:

```yaml
features:
  sandbox:
    extra:
      rw: [.]
```

## Hidden Modes

Modes with `hide: true` are excluded from `/mode` listing and `Tab` cycling but remain selectable via `/mode <name>`. A subset of these live in `.aura/config/modes/features/` — hidden modes used internally by Aura features. User-defined hidden modes can live anywhere in `.aura/config/modes/`.

The `default.md` mode is a hidden mode that serves as the base when no mode is explicitly set. It extends Ask mode with additional tool access (Write, Ask, Done, Gotify, Diagnostics, LspRestart, Speak, Transcribe, WebFetch, WebSearch).

To see the default modes that ship with `aura init`, inspect `.aura/config/modes/` after scaffolding.
