---
# EXAMPLE — This file is not loaded. Rename to <name>.md to use.
# Full reference of all mode frontmatter fields.

# Unique identifier for this mode. Referenced by agent "mode" field and /mode command.
name: Review

# Inherit config from one or more parent modes.
# Key absent in child = inherit from parent. Key present = replace parent's value.
inherit: [Ask]

# Human-readable description. Shown in /mode listing.
description: Read-only code review mode — can read and search but not modify files.

# Exclude from /mode listing and Shift+Tab cycling.
# Hidden modes can still be selected via /mode <name>.
hide: false

# Tool access control. Supports wildcard patterns (e.g., "mcp__*", "Todo*").
# Mode tool filters are combined with agent tool filters.
tools:
  # Patterns to enable. ["*"] = all tools. [] = all (default).
  enabled: ["*"]
  # Patterns to disable. Applied after enabled. Takes precedence.
  disabled:
    - Write
    - Mkdir
    - Patch
    - TodoCreate
    - TodoProgress
  # Tool policy. Controls which tool calls are auto-approved, require confirmation, or are blocked.
  # Patterns: "ToolName" (name-only) or "Bash:command*" (Bash argument matching). Supports * glob.
  # Precedence: deny > confirm > auto > default (auto).
  policy:
    auto:
      - Read
      - Glob
      - Grep
    confirm:
      - "Bash:go test*"
      - "mcp__*"
    deny:
      - "Bash:rm *"
      - "Bash:sudo *"
      - "Bash:git push*"
      - "Bash:git reset*"

# Hook filtering. Controls which hooks are active for this mode.
# Applied after agent-level hook filtering. Same pattern as tool filtering.
hooks:
  enabled: []
  disabled: ["go:format", "go:fix"]

# Per-mode feature overrides. Non-zero values are merged on top of agent features.
# Merge order: Global > Agent > Mode > Task = Effective Features.
features:
  compaction:
    threshold: 60
  tools:
    max_steps: 30
    read_before:
      write: true
  guardrail:
    mode: log
  subagent:
    max_steps: 10
---

You are in review mode. You can read, search, and analyze code — but you cannot modify files.

{{ if .Tools.Eager -}}
Tools available:
{{ range .Tools.Eager }}- {{ . }}
{{ end }}
{{ end -}}
{{ if .Tools.Deferred }}
{{ .Tools.Deferred }}
{{ end -}}

Focus on:

- Understanding the code structure and intent
- Identifying bugs, edge cases, and potential issues
- Suggesting improvements with concrete code examples
- Running existing tests to verify behavior

Provide specific, actionable feedback. Reference file paths and function names.
Do not attempt to edit files — suggest changes as code snippets in your response.
