---
# EXAMPLE — This file is not loaded. Rename to <name>.md to use.
# Full reference of all agent frontmatter fields.

# Unique identifier for this agent. Shown in /agent listing and status bar.
name: CodeReview

# Inherit config from one or more parent agents.
# Key absent in child = inherit from parent. Key present = replace parent's value.
# Slices are always replaced (no append). Use `disabled: []` to clear a parent's list.
# Merge order is left-to-right, child last.
# The `default` field is NOT inherited — it is an identity property.
inherit: [Base]

# Human-readable description. Shown in agent listings.
description: Code review agent — reads code, runs tests, provides feedback without editing.

# AI model configuration.
model:
  # Provider name. Must match a provider defined in providers/*.yaml.
  provider: anthropic
  # Model identifier at the provider.
  name: claude-sonnet-4-6
  # Extended thinking mode. Controls reasoning before responding.
  # Values: false/off (disabled), true (enabled), "low", "medium", "high" (effort level).
  think: medium
  # Max context window in tokens. Overrides the model's default context size.
  context: 128000
  # Optional generation parameters (sampling, output limits, thinking budget).
  # All fields use pointers — omitted fields inherit from CLI flags or provider defaults.
  generation:
    temperature: 0.3
    top_p: 0.9
    top_k: 40
    frequency_penalty: 0.0
    presence_penalty: 0.0
    max_output_tokens: 16000
    think_budget: 8000
    # stop: ["END", "---"]     # stop sequences (provider-dependent)
    # seed: 42                 # deterministic sampling seed (provider-dependent)

# Constrain LLM responses to a specific format (optional, nil = unconstrained text).
# Types: "text" (default), "json_object" (any valid JSON), "json_schema" (schema-constrained JSON).
# Note: Anthropic does NOT support json_object — use json_schema with a schema instead.
# response_format:
#   type: json_schema
#   name: review_output
#   schema:
#     type: object
#     properties:
#       issues:
#         type: array
#         items:
#           type: object
#           properties:
#             file: { type: string }
#             line: { type: integer }
#             severity: { type: string, enum: [error, warning, info] }
#             message: { type: string }
#           required: [file, severity, message]
#     required: [issues]
#     additionalProperties: false
#   strict: true

# How to handle thinking/reasoning blocks from prior conversation turns.
# Values: "" (keep as-is), "strip" (remove from history), "rewrite" (condense via thinking agent).
thinking: strip

# Tool access control. Supports wildcard patterns (e.g., "mcp__*", "Todo*").
tools:
  # Patterns to enable. ["*"] = all tools. [] = all (default).
  enabled: ["*"]
  # Patterns to disable. Applied after enabled. Takes precedence.
  disabled:
    - Write
    - Patch
    - Mkdir
    - mcp__*
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
    deny:
      - "Bash:rm *"
      - "Bash:sudo *"
      - "Bash:git push*"

# Hook filtering. Controls which hooks from config/hooks/*.yaml are active for this agent.
# Same pattern as tool filtering — supports * wildcards.
# When a hook is excluded and other hooks depend on it, dependents are cascade-pruned
# automatically (e.g., disabling "go:format" also prunes "go:fix" and "go:lint").
hooks:
  enabled: []
  disabled: ["go:*"]

# System prompt name. Must match a name in prompts/*.md.
# Values: "Agentic", "Chat", "Lite", or any custom prompt you define.
system: Agentic

# Default mode when this agent is active. Must match a name in modes/*.md.
# Values: "Edit", "Plan", "Ask", or any custom mode you define.
mode: Ask

# Exclude from /agent listing and Shift+Tab cycling.
# Hidden agents can still be selected via /agent <name> or --agent flag.
hide: false

# Mark this agent as the default when --agent is not specified.
# Only one agent may have default: true. Hidden agents can also be default.
# If no agent has default: true, the first non-hidden agent (alphabetical) is used.
default: false

# Mark this agent as available for the Task tool (subagent delegation).
# When true, the LLM can spawn this agent as a subagent via the Task tool.
# The agent remains fully usable as a normal agent via /agent, --agent, or Shift+Tab.
subagent: true

# Fallback agent chain. When this agent's provider fails permanently
# (after retries exhausted), Aura switches to the next agent in this list.
# Each entry is an agent name. Only the primary agent's fallback list is used —
# fallback agents' own fallback lists are ignored. Failover is one-way per session.
fallback:
  - openrouter/gpt-oss-120b
  - ollama/high

# Controls which AGENTS.md files are injected into this agent's prompt.
# Values: "" or "all" (default = inject all), "global", "local", "none".
agentsmd: local

# Files to load into the system prompt. Paths resolved relative to config home.
# Expanded as Go templates before reading — supports path variables and conditionals.
files:
  - docs/review-guidelines.md
  - CONVENTIONS.md
  # Conditional: only loaded when env var is set
  # - "{{ if env \"EXTRA_RULES\" }}config/prompts/extra-rules.md{{ end }}"
  # From global config home
  # - "{{ .Config.Global }}/shared/coding-standards.md"

# Feature overrides. Merged on top of global features/*.yaml defaults.
# Only set fields you want to override — unset fields keep global values.
features:
  compaction:
    threshold: 70
    keep_last_messages: 5
    agent: Compaction
  tools:
    max_steps: 30
    read_before:
      write: true
  sandbox:
    enabled: true
    extra:
      ro:
        - /var/log
---

You are a code review agent. Your job is to analyze code quality, find bugs, and suggest improvements — without making edits.

{{ if .Tools.Eager -}}
Tools available:
{{ range .Tools.Eager }}- {{ . }}
{{ end }}
{{ end -}}

Review guidelines:

- Focus on correctness, readability, and maintainability
- Flag potential bugs, race conditions, and edge cases
- Suggest concrete improvements with code snippets
- Note any missing error handling or test coverage
- Be specific: reference file paths and function names

{{ if .Sandbox.Enabled -}}
You are running in a sandboxed environment with restricted filesystem access.
{{ end -}}

When reviewing, read the code first, then run existing tests to understand current coverage before providing feedback.
