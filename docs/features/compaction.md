---
layout: default
title: Compaction
parent: Features
nav_order: 5
---

# Context Compaction

Compaction is enabled by default. When the conversation fills the context window, older messages are compressed into a summary, freeing space for new content.

## How It Works

1. **Synthetic trim** — At 50% context fill (configurable), duplicate synthetic messages are removed to delay compaction.
2. **Auto-compaction** — At 80% context fill (configurable), compaction triggers automatically.
3. **Summarize** — Older messages are preprocessed (thinking blocks removed, tool results truncated) and sent to the compaction agent, which generates a summary preserving structured data while compressing narrative.
4. **Rebuild** — History is reconstructed: system prompt + compaction summary + most recent messages.

## What Gets Preserved

- Requirement checklists and acceptance criteria — reproduced verbatim
- File paths, package names, and dependency choices
- Explicit decisions and rationale stated in conversation
- Todo state — mechanically appended to the summary

## Manual Trigger

Use `/compact` to trigger compaction manually at any time.

## Configuration

| Setting                  | Default    | Description                                                      |
| ------------------------ | ---------- | ---------------------------------------------------------------- |
| `threshold`              | 80         | Context fill % that triggers auto-compaction                     |
| `max_tokens`             | 0          | Absolute token count trigger (overrides `threshold` when set)    |
| `trim_threshold`         | 50         | Fill % for synthetic message trimming                            |
| `trim_max_tokens`        | 0          | Absolute token count trigger for trimming                        |
| `keep_last_messages`     | 10         | Messages preserved during compaction                             |
| `chunks`                 | 1          | Chunks for sequential compaction (1 = single-pass)               |
| `agent`                  | Compaction | Agent for generating summaries                                   |
| `prompt`                 |            | Named prompt for self-compaction (overrides `agent`)             |
| `tool_result_max_length` | 200        | Max chars for tool results in compaction messages                |
| `prune.mode`             | off        | When to prune old tool results: `off`, `iteration`, `compaction` |
| `prune.protect_percent`  | 30         | % of context window to protect from pruning                      |
| `prune.arg_threshold`    | 200        | Min estimated tokens for tool call args to be prunable           |

`max_tokens` takes priority over `threshold` when set; same for `trim_max_tokens` vs `trim_threshold`. The agent's `context:` field sets the effective context window size and takes priority over provider-reported values.

See [Compaction Config]({{ site.baseurl }}/configuration/features#compaction) for the full YAML.

## Per-Agent Overrides

Agents override compaction via `features.compaction` in their frontmatter. Resolution order:

1. `prompt` set → **self-compact**: current agent's model with the named prompt. Dedicated compaction agent bypassed.
2. `agent` set → use that dedicated agent.
3. Neither → use the default agent from `compaction.yaml`.
4. No agent or prompt at all → **prune-only**: mechanical pruning without LLM summarization.

```yaml
# Self-compact
features:
  compaction:
    prompt: "Compaction"

# Different dedicated agent
features:
  compaction:
    agent: "FastCompactor"

# Tweak thresholds only
features:
  compaction:
    threshold: 95
    keep_last_messages: 20
```

## Chunked Compaction

When `chunks` is set to N > 1, compactable messages are split into N chunks and compacted sequentially — each chunk's summary feeds into the next, producing a coherent result that preserves more detail than a single pass. Chunk boundaries respect tool call/result pairs. Todo state is only included in the last chunk's prompt.

## Progressive Retry

Compaction retries with progressively shorter tool result content if the summary is too large (`200 → 150 → 100 → 50 → 0` chars), then with progressively lower `keep_last_messages` down to 0. If context is still exceeded, a warning is shown suggesting `/compact` or a new session.

## Plugin Hooks

**`BeforeCompaction`** fires before compaction; skip by returning `sdk.Result{Compaction: &sdk.CompactionModification{Skip: true}}`. Context: `Forced`, `TokensUsed`, `ContextPercent`, `MessageCount`, `KeepLast`. **`AfterCompaction`** fires after completion (read-only). Context: `Success`, `PreMessages`, `PostMessages`, `SummaryLength`. Neither hook fires on the prune-only path. See [Plugins]({{ site.baseurl }}/features/plugins#hook-timings).

## Pruning

Pruning removes low-value tool call arguments from older messages to reclaim context space without summarizing. Three modes: `off` (default), `iteration` (after each tool-use loop), `compaction` (during compaction). Only args exceeding `arg_threshold` estimated tokens are candidates. The most recent messages covering `protect_percent` of the context window are never touched.

```yaml
compaction:
  prune:
    mode: "off"
    protect_percent: 30
    arg_threshold: 200
```
