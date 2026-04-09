---
layout: default
title: Thinking
parent: Features
nav_order: 6
---

# Extended Thinking

Aura supports extended thinking/reasoning, allowing models to reason before responding.

## Thinking Levels

| Level           | Description                                   |
| --------------- | --------------------------------------------- |
| `off` / `false` | No thinking — direct response                 |
| `on` / `true`   | Enable thinking at the default level (medium) |
| `low`           | Minimal reasoning                             |
| `medium`        | Moderate reasoning                            |
| `high`          | Maximum reasoning effort                      |

Higher thinking levels consume more output tokens. The thinking content counts toward the context window but is managed by the strip/rewrite strategies below.

Set per-agent in the agent frontmatter via `model.think`.

## Controls

| Method                        | Description                                             |
| ----------------------------- | ------------------------------------------------------- |
| `/think [level]` or `/effort` | Set thinking level (off, on, low, medium, high)         |
| `Ctrl+R`                      | Toggle thinking off ↔ on (true)                         |
| `Ctrl+E`                      | Cycle all levels (off → on → low → medium → high → off) |
| `--think` flag                | Set starting level from CLI                             |

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
