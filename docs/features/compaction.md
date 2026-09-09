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
| `timeout`                | 0s         | Total compaction/recovery deadline; 0 uses only the caller deadline |
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

Overflow recovery requires the estimated request size to decrease, not merely its
message count. It can reduce the preserved tail to zero; zero preserves no
non-system messages. An ineffective zero-tail attempt or a skipped recovery stops
with an error. At most three consecutive provider-overflow recoveries are allowed
without a successful main response; cancellation stops recovery immediately.

Compaction retries with progressively shorter tool result content **only for context-overflow errors**
(`200 → 150 → 100 → 50 → 0` chars), then with progressively lower `keep_last_messages` down to 0.
Empty summaries, attempted tool calls, authorization failures and exhausted transport retries stop that recovery;
shrinking the transcript does not correct those failures. Tools emitted by a compactor are never executed.
Set `truncation_retries: []` to disable the inner size-retry sequence.

`timeout` bounds the whole recovery, including all chunks and retries. It cannot extend the parent task deadline.
For a short automated assessment, a single chunk and a bounded timeout avoid multiplying summarization calls.

After compaction, Aura also attaches actual current-turn tool receipts outside the model-generated summary.
They list executed tools and their returned responses (bounded excerpts for long outputs), including suppressed
or failed actions. These receipts take precedence over summary claims; they do not cover earlier turns or prove
an external effect beyond what the tool response confirms. The usual summary still carries interpretations and
evidence; it is not an authoritative delivery ledger.

## Plugin Hooks

**`BeforeCompaction`** fires before compaction; skip by returning `sdk.Result{Compaction: &sdk.CompactionModification{Skip: true}}`. Context: `Forced`, `TokensUsed`, `ContextPercent`, `MessageCount`, `KeepLast`. **`AfterCompaction`** fires after completion (read-only). Context: `Success`, `PreMessages`, `PostMessages`, `SummaryLength`. Neither hook fires on the prune-only path. See [Plugins]({{ site.baseurl }}/features/plugins#hook-timings).

## Pruning

Pruning removes old tool results and large tool call arguments to reclaim context space without summarizing. Three modes: `off` (default), `iteration` (after each tool-use loop), `compaction` (during compaction). Only arguments exceeding `arg_threshold` estimated tokens are candidates. The most recent messages covering `protect_percent` of the context window, including the message crossing that boundary, are protected. The latest unanswered tool-call batch and all its results remain intact until an assistant response has assessed them.

```yaml
compaction:
  prune:
    mode: "off"
    protect_percent: 30
    arg_threshold: 200
```
