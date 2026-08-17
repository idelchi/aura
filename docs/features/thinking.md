---
layout: default
title: Thinking
parent: Features
nav_order: 6
---

# Extended Thinking

Aura supports extended thinking/reasoning, allowing models to reason before responding.

## Thinking Modes and Efforts

| Value                  | Description                                                   |
| ---------------------- | ------------------------------------------------------------- |
| `off` / `false`        | Request that thinking be disabled, where the model supports it |
| `on` / `true` / `auto` | Use the provider or model's default thinking behavior          |
| `none`                 | Request no reasoning from providers that expose it as an effort |
| `minimal`              | Minimal explicit effort                                       |
| `low`                  | Low explicit effort                                           |
| `medium`               | Medium explicit effort                                        |
| `high`                 | High explicit effort                                          |
| `xhigh`                | Extra-high explicit effort                                    |
| `max`                  | Highest provider-defined effort                               |

These are Aura's provider-neutral values, not a promise that every model supports every effort. Aura validates an explicit effort after resolving the provider and model, and reports the supported set when a value is unavailable. Higher efforts generally consume more output tokens. Thinking content counts toward the context window but is managed by the strip/rewrite strategies below.

For Ollama, [most thinking models](https://docs.ollama.com/capabilities/thinking) accept booleans or `low`, `medium`, `high`, and `max`. GPT-OSS is the documented exception: it accepts only `low`, `medium`, and `high`; booleans are ignored and its trace cannot be fully disabled.

Set per-agent in the agent frontmatter via `model.think`.

For example, start Aura at the highest effort only when the selected model advertises it:

```sh
aura --think max
```

## Controls

| Method                         | Description                                                    |
| ------------------------------ | -------------------------------------------------------------- |
| `/think [value]` or `/effort`  | Set any mode or provider-supported explicit effort listed above |
| `Ctrl+R`                       | Toggle thinking off ↔ auto                                     |
| `Ctrl+E`                       | Cycle common values: off → auto → low → medium → high → off   |
| `--think <value>`              | Set the starting mode or effort from the CLI                    |

## Thinking Display

Thinking blocks are rendered in faint gray in the TUI. Toggle visibility:

- `/verbose` command
- `Ctrl+T` keybinding

## Prior Turn Handling

Each agent configures how thinking blocks from prior conversation turns are handled:

| Mode           | Description                                                        |
| -------------- | ------------------------------------------------------------------ |
| `""` (default) | Keep thinking blocks as-is in history                              |
| `"strip"`      | Strip thinking from older messages that exceed the token threshold |
| `"rewrite"`    | Condense older thinking via a dedicated agent or self-rewrite      |

Set in agent frontmatter: `thinking: "strip"`

Both `strip` and `rewrite` respect configurable thresholds in `features/thinking.yaml`:

```yaml
thinking:
  agent: Thinking # dedicated agent for rewriting
  prompt: "" # named prompt for self-rewrite (overrides agent)
  keep_last: 5 # recent messages whose thinking is preserved
  token_threshold: 300 # minimum tokens for a block to be affected
```

- `keep_last` — the N most recent messages always keep their thinking blocks unchanged.
- `token_threshold` — blocks below this threshold are left alone (small thinking blocks are cheap to keep).
- `prompt` — when set, uses the current agent's model with a named system prompt instead of delegating to a separate agent. Mirrors compaction's self-compact pattern.

Agent resolution uses the same pattern as compaction — see [Feature Agent Resolution]({{ site.baseurl }}/configuration/features#feature-agent-resolution).

If `thinking: rewrite` is configured and neither agent nor prompt can be resolved, it fails with an error. No silent fallback to stripping.
