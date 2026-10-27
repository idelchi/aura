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
3. **Summarize** — Older messages are preprocessed (thinking blocks removed, tool results bounded with explicit markers) and sent to the compaction agent, which summarizes only what that transcript establishes.
4. **Rebuild** — History is reconstructed: system prompt + compaction summary + most recent messages.

## What Gets Preserved

- Requirements, constraints, decisions and corrections as the conversation states them; a later correction replaces
  what it corrects
- Identifiers, paths, values and quotations only where the transcript shows them in full
- Todo state — mechanically appended to the summary

User and assistant text is sent to the summarizer unshortened. Each tool result keeps at most
`tool_result_max_length` bytes. A longer result ends in a marker such as
`[tool result excerpt: first 200 of 18097 bytes shown; the remaining 17897 bytes are absent from this transcript]`,
cut on a UTF-8 character boundary. A length of zero or less replaces every non-empty result with
`[tool result omitted: all 18097 bytes are absent from this transcript]`. The summarization instruction forbids
completing schemas, listings, code, JSON or quotations from partial evidence and asks the summary to name the source to
re-read instead. An instruction does not guarantee that a model complies; summary faithfulness has to be evaluated on
actual outputs. The instruction is the same for single-pass and chunked compaction; the configured compaction prompt
still decides what to retain.

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
| `tool_result_max_length` | 200        | Max bytes per tool result sent to the summarizer; <= 0 omits them |
| `truncation_retries`     | 150, 100, 50, 0 | Smaller lengths for over-budget requests and provider overflow |
| `prune.mode`             | off        | When to prune old tool results: `off`, `iteration`, `compaction` |
| `prune.protect_percent`  | 30         | % of context window to protect from pruning                      |
| `prune.arg_threshold`    | 200        | Min estimated tokens for tool call args to be prunable           |

`max_tokens` takes priority over `threshold` when set; same for `trim_max_tokens` vs `trim_threshold`. The agent's `context:` field sets the effective context window size and takes priority over provider-reported values.

Automatic compaction runs before a model request needs the space, not after a completed answer. A following `/new`
or process exit therefore does not pay for a discarded summary. Explicit `/compact` and bounded overflow recovery
are unchanged. Compaction time includes failed attempts as well as successful summaries.

The recent-message count excludes internal UI bookkeeping such as bookmarks. Tool calls and their results remain
together when choosing the preserved tail. Before accepting new input, Aura rechecks the input budget after compaction;
a summary that leaves too little space is an admission failure. Automation, including `/until`, stops on that error
instead of retrying rejected input. Interactive sessions stay open so you can compact, clear context or revise the input.

Local token estimates are calibrated upward by observed provider input overhead, including chat templates not
represented in stored messages. This remains an estimate, not an exact tokenizer; native context-overflow recovery
is still required. Changing models or resetting token state clears the calibration.

See [Compaction Config]({{ site.baseurl }}/configuration/features#compaction) for the full YAML.

## Per-Agent Overrides

Agents override compaction via `features.compaction` in their frontmatter. Resolution order:

1. `prompt` set → **self-compact**: current agent's model with the named prompt. Dedicated compaction agent bypassed.
2. `agent` set → use that dedicated agent.
3. Neither → use the default agent from `compaction.yaml`.
4. No agent or prompt at all → **prune-only**: mechanical pruning without LLM summarization.

Self-compaction inherits the active agent's current `model.think` and `model.generation` settings,
including thinking budgets, sampling controls and output limits. A dedicated compaction agent uses
its own settings. These apply to every chunk and overflow retry; unset values remain unset.
Compaction uses its summary prompt and sends no tools.

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

### Summarization request budget

Each summarization request — the single pass, or each chunk — is admitted against an estimated input budget before it
is sent. The budget reuses the conversation's own compaction trigger, applied to the summarizer's context window:
`max_tokens` when set and smaller than that window, otherwise `threshold` percent of the window. A `max_tokens` that
reaches or exceeds a smaller summarizer window would leave no room for the summary, so that window's `threshold`
percentage applies instead. A `threshold` above 100 (automatic compaction effectively disabled) is capped at the window
and leaves no headroom; a `threshold` at or below 0 cannot derive a budget and is a configuration error. When the
summarizer's agent sets a positive `max_output_tokens`, that many tokens then stay free within the window. Nonpositive
provider sentinel values remain unchanged on the wire and are not treated as finite output caps. The budget limits only
what is sent; it never caps the summary, its reasoning or the task. A `max_output_tokens` that leaves no input room is
a configuration error. With neither a known window nor `max_tokens`, no budget is applied and only provider overflow
handling remains. The window is the summarizer agent's configured `context`, otherwise the context the provider reports
for the model.

The request uses the first length in `tool_result_max_length`, then `truncation_retries`, whose estimate fits. Larger
lengths that do not fit are skipped without a provider call. If the provider still reports a context overflow, the
request moves on to the next length. For providers exposing truncation and context-shift controls (Ollama), compaction
disables both so overflow is rejected rather than silently removing source. Any other failure — an empty summary,
attempted tool calls, authorization,
exhausted transport retries, cancellation — stops at once. Tools emitted by a compactor are never executed. Chunks
choose their lengths independently, so one large tool result does not shorten the evidence of every other chunk.

A summary is accepted only if the provider does not report it cut off. When the provider reports that generation
stopped at a length limit (`finish` `length`, normalized from llama.cpp/OpenAI-compatible `finish_reason`, Ollama
`done_reason` and the Fantasy-based providers), the summary may be incomplete and never replaces the history. If the
summarizer agent's `max_output_tokens` was used up, compaction fails at once, because less input would not help.
Otherwise the request moves on to the next length, which leaves more room; when the lengths run out it fails with
`ErrCompactionSummaryCutOff`. Recovery then stops instead of shortening the preserved tail. A provider that reports no
finish reason is accepted as before; that is not proof of completeness. Reported errors, content filtering and tool-call
termination are rejected even when the provider supplies text without actual tool calls. The debug log records the
finish reason of each accepted summary. Ordinary chat responses are unaffected by this check.

User and assistant text is never shortened. When even the last configured length does not fit, compaction fails
with `ErrCompactionInputBudget`, naming the estimate, the budget and how the budget was derived. The conversation is
unchanged. Recovery does not retry with a shorter preserved tail, because that would only enlarge the request. Raise
`chunks`, clear context or start a new conversation. A provider overflow at the last length keeps the earlier
behaviour: recovery then also reduces `keep_last_messages` towards 0. Set `truncation_retries: []` to use only
`tool_result_max_length`.

Estimates use the session's token estimator on the rendered request plus the observed provider input overhead. They
are approximations, not the provider's tokenizer, so provider overflow handling remains in place.

`timeout` bounds the whole recovery, including all chunks and retries. It cannot extend the parent task deadline.
For a short automated assessment, a single chunk and a bounded timeout avoid multiplying summarization calls.

After compaction, Aura also attaches actual current-turn tool receipts outside the model-generated summary.
They list executed tools and their returned responses, including suppressed or failed actions. A response longer than
200 bytes is shown as an excerpt ending in `[receipt excerpt: first 200 of N bytes; the rest is not shown]`. This
fixed size is independent of `tool_result_max_length`, so giving the summarizer full evidence does not enlarge the
rebuilt context. These receipts take precedence over summary claims; they do not cover earlier turns or prove
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
