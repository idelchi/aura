---
layout: default
title: Guardrail
parent: Features
---

# Guardrail

Secondary LLM that validates tool calls and user messages against a safety policy before they execute or enter the conversation.

## Overview

Two independent scopes control what gets checked:

- **tool_calls** — validates tool calls before execution (after tool policy check)
- **user_messages** — validates user input before it enters the conversation

Each scope has its own agent and/or prompt, allowing different models or policies for each.

## Modes

| Mode         | Behavior                                               |
| ------------ | ------------------------------------------------------ |
| `""` (empty) | Disabled (default)                                     |
| `"log"`      | Shows a flagged notice in the UI but proceeds normally |
| `"block"`    | Rejects the tool call or user message                  |

## Configuration

**File:** `.aura/config/features/guardrail.yaml`

```yaml
guardrail:
  mode: block # "block", "log", or "" (disabled)
  on_error: allow # "block" (fail-closed) or "allow" (fail-open)
  timeout: 2m # Max duration per check. Default: 2m when enabled.

  scope:
    tool_calls:
      agent: "GuardRail:Tool"
      # prompt: guardrail-tool   # self-guardrail (uses current agent's model)
    user_messages:
      agent: "GuardRail:Input"
      # prompt: guardrail-input

  tools: # Which tool calls trigger guardrail checks
    enabled: [] # empty = all
    disabled: [] # applied after enabled
```

## Agent vs Prompt

- **Agent** (`agent:`) — dedicated hidden agent with its own provider and model, runs independently of the conversation model.
- **Prompt** (`prompt:`) — self-guardrail mode, uses the current agent's model with a named system prompt from `prompts/system/`.

If both are set, `prompt` takes precedence.

## Response Protocol

The guardrail request uses `response_format: json_schema` to force structured output:

```json
{ "result": "safe" }
{ "result": "unsafe", "reason": "Command attempts to delete system files" }
```

`result` is required (`safe` or `unsafe`). `reason` is optional — when provided, it is included in block/log messages for better diagnostics.

Parsing order: JSON parse (primary) → first-token parse (fallback for providers that ignore `ResponseFormat`). Any unrecognized response is treated as unsafe (fail-closed).

## Error Policy

`on_error` controls what happens when the guardrail check itself fails (timeout, network error, model unavailable). Independent of `mode`.

| `on_error` | Behavior                                 |
| ---------- | ---------------------------------------- |
| `"block"`  | Fail-closed. Default when `mode: block`. |
| `"allow"`  | Fail-open. Default when `mode: log`.     |

Set explicitly to decouple error behavior from policy — e.g., `mode: block` + `on_error: allow` blocks unsafe content but doesn't paralyze the session when the guardrail provider is down.

## Integration Points

**Tool calls:** Runs after tool policy check, before execution. If blocked, the tool call returns an error result and the assistant continues. In log mode, a DisplayOnly message is added (visible in UI and history, never sent to the LLM).

**Batch sub-calls:** Each sub-call in a [Batch]({{ site.baseurl }}/features/tools#batch) is individually checked. A blocked sub-call returns an error while others proceed independently.

**User messages:** Runs after compaction and input size checks. If blocked, the message is rejected with a user-facing notice.

## Per-Agent Override

```yaml
features:
  guardrail:
    mode: log
    on_error: block
    scope:
      tool_calls:
        prompt: guardrail-tool
```

## Default Agents

- **GuardRail:Tool** — classifies tool calls against sandbox policy (path traversal, destructive commands, data exfiltration, indirect execution)
- **GuardRail:Input** — classifies user messages for sensitive content (API keys, tokens, passwords, credentials)

Both are hidden agents in `.aura/config/agents/features/guardrail/`.
