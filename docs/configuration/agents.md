---
layout: default
title: Agents
parent: Configuration
nav_order: 1
---

{% raw %}

# Agents

An agent is a named configuration that binds a model, provider, system prompt, default mode, and tool access together. Each agent is a Markdown file with YAML frontmatter in `.aura/config/agents/`.

When you switch agents (via `/agent`, `Shift+Tab`, or `--agent`), Aura swaps the entire LLM configuration in one step.

Use agents to switch models, providers, or personas. Use modes for same-model behavior variants — read-only vs editing, planning vs execution. Agents change who works; modes change how.

## Frontmatter Schema

```yaml
---
name: AgentName # Unique identifier
description: "" # Shown in /agent listing
inherit: [Base] # Parents. Absent = inherit; present = replace. Slices replaced.

model:
  provider: ollama # Provider name (must match providers/*.yaml)
  name: llama3:8b # Model identifier
  think: high # false/off, true, "low", "medium", "high"
  context: 65536 # Context window in tokens
  generation: # All pointer fields — omit to inherit
    temperature: 0.7
    top_p: 0.95
    top_k: 40
    frequency_penalty: 0.0
    presence_penalty: 0.0
    max_output_tokens: 8000
    stop: []
    seed: 42
    think_budget: 4000

response_format: # Constrain LLM output format (optional)
  type: json_schema # "text" (default), "json_object", or "json_schema"
  name: my_output # Schema name (required for json_schema)
  schema: # JSON Schema as YAML
    type: object
    properties:
      answer: { type: string }
    required: [answer]
    additionalProperties: false
  strict: true

thinking: "" # Prior thinking: "" (keep), "strip", "rewrite"

tools:
  enabled: ["*"] # Glob patterns to enable (["*"] = all)
  disabled: [] # Applied after enabled
  policy:
    auto: [] # Auto-approve without prompting
    confirm: [] # Require user approval
    deny: [] # Hard-block
      # Pattern syntax: "ToolName", "Bash:command*", "Tool:/path/*"

hooks:
  enabled: [] # Hook name patterns to include ([] = all)
  disabled: ["go:*"] # Hook name patterns to exclude. Cascade-prunes dependents.

system: Agentic # System prompt name (Agentic, Chat, Lite)
mode: Plan # Default mode (Edit, Plan, Ask)

hide: false # Exclude from /agent listing and Shift+Tab
default: false # Use when --agent not specified (one allowed)
subagent: false # Available for Task tool delegation
agentsmd: all # AGENTS.md injection: "all", "global", "local", "none"

fallback: # Provider failover chain (ordered agent names)
  - openrouter/gpt-oss-120b
  - anthropic/sonnet

files: # Files injected into system prompt
  - docs/style-guide.md # Relative to config home (.aura/)

features: # Override global feature defaults (deep merge)
  compaction:
    threshold: 70
  tools:
    max_steps: 20
---
```

The Markdown body below the frontmatter becomes the agent's prompt template. The `system` field selects a system prompt from `prompts/system/`.

## Inheritance

```yaml
inherit: [Base]              # Single parent
inherit: [Base, Restricted]  # Multiple parents — merged left-to-right, child last
```

Key absent in child = inherit from parent. Key present = replace entirely. Slices are always replaced. The `default` field is excluded from inheritance.

The prompt body also inherits — last non-empty body in the chain wins. Cycles and missing parents produce errors at startup.

## File Autoloading

The `files:` field lists paths to inject into the system prompt. Paths and file contents are both rendered as Go templates before loading.

```yaml
files:
  - docs/style-guide.md
  - "{{ .Config.Source }}/shared/rules.md"
  - '{{ if env "LOAD_EXTRA" }}config/prompts/extra.md{{ end }}'
```

Paths resolve relative to `.aura/`. Conditional paths evaluating to empty are silently skipped. Missing files cause an error.

Template variables available in paths and prompt bodies: `.Config.Global`, `.Config.Project`, `.Config.Source`, `.LaunchDir`, `.WorkDir`, `.Model.Name`, `.Provider`, `.Agent`, `.Mode.Name`, `.Tools.Eager`, `.Memories.Local`, `.Memories.Global`, `{{ env "VAR" }}`, `{{ index .Vars "key" }}`. See [Prompts]({{ site.baseurl }}/configuration/prompts) for the full set.

## AGENTS.md Injection

| Value           | Behavior                                        |
| --------------- | ----------------------------------------------- |
| `""` or `"all"` | Inject all discovered AGENTS.md files (default) |
| `"global"`      | Only global-scoped AGENTS.md (`~/.aura/`)       |
| `"local"`       | Only project and walked-directory AGENTS.md     |
| `"none"`        | Skip all injection                              |

## Provider Failover

The `fallback:` list is tried in order when the primary provider fails permanently (after retries). Each fallback is a full agent with its own model, prompt, and tools.

Triggers on: network failures, auth errors, rate limits, credit exhaustion, model unavailable, server errors. Content filter errors and user cancellation do not trigger failover.

Failover is one-way per session — use `/agent <name>` to switch back.

## Feature Overrides

The `features:` block overrides global defaults from `features/*.yaml`. Unset fields inherit the global value.

Override precedence: **global → CLI flags → agent → mode → task**

Available keys: `compaction`, `title`, `thinking`, `vision`, `embeddings`, `tools`, `stt`, `tts`, `sandbox`, `subagent`, `plugins`, `mcp`, `estimation`, `guardrail`. See [Features]({{ site.baseurl }}/configuration/features).

## Example

```yaml
---
name: MyAgent
model:
  provider: ollama
  name: llama3:8b
  think: medium
  context: 16384
  generation:
    temperature: 0.7
    top_k: 40
    max_output_tokens: 8000
thinking: strip
tools:
  enabled: ["*"]
  disabled: ["Bash"]
hooks:
  disabled: ["go:*"]
features:
  tools:
    max_steps: 100
  compaction:
    threshold: 90
system: Agentic
mode: Edit
---
```

{% endraw %}
