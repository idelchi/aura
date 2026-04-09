---
layout: default
title: Features
parent: Configuration
nav_order: 4
---

{% raw %}

# Feature Configuration

Features are configurable capabilities with their own YAML config files in `.aura/config/features/`. Features that need LLM calls use a dedicated hidden agent defined in `.aura/config/agents/features/`.

## Feature Agent Resolution

Several features delegate work to an LLM. Compaction, Thinking, Guardrail, and Title support two resolution patterns:

1. **`agent:`** — Dedicated hidden agent with its own model/provider. Separate API call.
2. **`prompt:`** — Reuses the current agent's model with a named system prompt from `prompts/`. No extra provider config.

Resolution order: `prompt:` wins over `agent:`. If neither is set: Compaction falls back to prune-only, Title falls back to first user message, others error.

| Feature                                | `agent:` | `prompt:` | Notes                                                                          |
| -------------------------------------- | -------- | --------- | ------------------------------------------------------------------------------ |
| Compaction                             | Yes      | Yes       | Falls back to prune-only if neither configured                                 |
| Thinking                               | Yes      | Yes       | Only runs when thinking blocks cross `keep_last` boundary                      |
| Guardrail                              | Yes      | Yes       | Per-scope: `scope.tool_calls.agent/prompt`, `scope.user_messages.agent/prompt` |
| Title                                  | Yes      | Yes       | Falls back to first user message if neither configured                         |
| Vision, STT, TTS, Embeddings, Reranker | Yes      | No        | Agent name provides model/provider — no runtime resolution                     |

## Compaction

**File:** `features/compaction.yaml` — See [Compaction]({{ site.baseurl }}/features/compaction)

```yaml
compaction:
  threshold: 80 # context fill % that triggers compaction (1-100)
  # max_tokens: 32000    # absolute token trigger (overrides threshold)
  trim_threshold: 50 # fill % at which synthetic messages are trimmed first (must be < threshold)
  # trim_max_tokens: 16000
  keep_last_messages: 10
  chunks: 1 # 1 = single-pass; N splits history into N sequential chunks
  agent: Compaction
  # prompt: ""           # named prompt for self-compaction (overrides agent)
  tool_result_max_length: 200
  truncation_retries: [150, 100, 50, 0]
  prune:
    mode: "off" # "off", "iteration", "compaction"
    protect_percent: 30 # % of context to protect from pruning
    arg_threshold: 200 # min tokens for tool call args to be prunable
```

## Embeddings

**File:** `features/embeddings.yaml` — See [Embeddings]({{ site.baseurl }}/features/embeddings)

```yaml
embeddings:
  agent: "Embeddings"
  max_results: 5
  gitignore: | # gitignore-style patterns for indexed files
    *
    !*/
    !src/**/*.go
  offload: false # unload embedding model after use (frees VRAM for reranker)
  chunking:
    strategy: "auto" # auto, ast, line
    max_tokens: 500
    overlap_tokens: 75
  reranking:
    agent: "Reranker"
    multiplier: 4 # fetch max_results * multiplier candidates, rerank down
    offload: false
```

## Thinking

**File:** `features/thinking.yaml` — See [Thinking]({{ site.baseurl }}/features/thinking)

```yaml
thinking:
  agent: "Thinking"
  # prompt: ""           # named prompt for self-rewrite (overrides agent)
  keep_last: 5 # most recent messages whose thinking blocks are always preserved
  token_threshold: 300 # min tokens for a block to be affected by strip/rewrite
```

## Vision

**File:** `features/vision.yaml`

```yaml
vision:
  agent: "Vision" # must support vision
  dimension: 1024 # max pixel dimension for compression
  quality: 75 # JPEG quality (1-100)
```

## Speech-to-Text (STT)

**File:** `features/stt.yaml`

```yaml
stt:
  agent: "Transcribe" # provider must support /v1/audio/transcriptions
  language: "" # ISO-639-1 hint (e.g. "en"). Empty = auto-detect
```

## Text-to-Speech (TTS)

**File:** `features/tts.yaml`

```yaml
tts:
  agent: "Speak" # provider must support /v1/audio/speech
  voice: "alloy" # voice identifier (server-dependent)
  format: "mp3" # mp3, opus, aac, flac, wav, pcm
  speed: 1.0 # 0.25-4.0
```

## Title

**File:** `features/title.yaml`

```yaml
title:
  disabled: false
  agent: "Title"
  prompt: "" # named prompt for self-title (overrides agent)
  max_length: 50
```

## Tools

**File:** `features/tools.yaml` — See [Tools]({{ site.baseurl }}/features/tools)

```yaml
tools:
  mode: percentage # "tokens" or "percentage" guard mode
  result:
    max_tokens: 20000 # max tokens per result (mode: tokens)
    max_percentage: 95 # max context fill after result (mode: percentage)
  read_small_file_tokens: 2000
  max_steps: 50
  token_budget: 0 # cumulative input+output limit; 0 = disabled
  rejection_message: >-
    Error: Tool result too large (%d tokens, limit %d).
    Try a more specific query or use offset/limit parameters.
  bash:
    truncation:
      max_output_bytes: 1048576 # 1MB per stream; 0 = disabled
      max_lines: 200
      head_lines: 100
      tail_lines: 80
    rewrite: "" # Go text/template rewriting Bash commands; receives {{ .Command }}
  webfetch_max_body_size: 5242880
  # parallel: false      # omit or true = parallel tool calls (default)
  user_input_max_percentage: 80 # max context fill after user message; 0 = disabled
  read_before:
    write: true # require read before overwriting files
    delete: false # require read before deleting files
  enabled: [] # default include patterns (glob); empty = all
  disabled: ["mcp__*"] # default exclude patterns (glob)
  policy: # global default tool policy (additive baseline for agent/mode policies)
    auto: [] # tool/Bash patterns auto-approved
    confirm: [] # tool/Bash patterns requiring user approval
    deny: [] # tool/Bash patterns hard-blocked
  opt_in: [] # tools hidden unless explicitly named
  deferred: [] # tools excluded from request, loaded on demand via LoadTools
```

## Sandbox

**File:** `features/sandbox.yaml` — See [Sandboxing]({{ site.baseurl }}/features/sandboxing)

```yaml
sandbox:
  enabled: true
  restrictions:
    ro: [] # read-only paths (support $HOME expansion)
    rw: [] # read-write paths
  # extra:    # additive paths from agent/mode frontmatter
  #   ro: []
  #   rw: []
```

## Subagent

**File:** `features/subagent.yaml`

```yaml
subagent:
  max_steps: 25
  default_agent: "" # agent for Task tool calls; empty = parent agent
```

## Plugins

**File:** `features/plugins.yaml` — See [Plugins]({{ site.baseurl }}/features/plugins)

```yaml
plugins:
  dir: "" # plugin directory relative to config home; empty = "plugins/"
  unsafe: false # allow plugins compiled without safety checks
  include: [] # load only matching names; empty = all
  exclude: [] # skip matching names (applied after include)
  config:
    global: {} # key-value pairs sent to ALL plugins
    local: {} # per-plugin overrides keyed by plugin name
```

## MCP

**File:** `features/mcp.yaml` — See [MCP]({{ site.baseurl }}/features/mcp)

```yaml
mcp:
  enabled: [] # include patterns (glob); empty = all servers
  disabled: [] # exclude patterns (glob)
```

## Guardrail

**File:** `features/guardrail.yaml` — See [Guardrail]({{ site.baseurl }}/features/guardrail)

```yaml
guardrail:
  mode: "" # "block", "log", "" (disabled)
  # on_error: ""         # "block" (fail-closed) or "allow" (fail-open); inherits from mode
  timeout: 2m
  scope:
    tool_calls:
      agent: ""
      prompt: ""
    user_messages:
      agent: ""
      prompt: ""
  tools:
    enabled: [] # only check matching tools (empty = all)
    disabled: [] # skip matching tools
```

## Estimation

**File:** `features/estimation.yaml`

```yaml
estimation:
  # rough = chars/divisor | tiktoken = tiktoken encoding
  # rough+tiktoken = max of both (default) | native = provider tokenization
  method: "rough+tiktoken"
  divisor: 4 # chars-per-token for rough estimation
  encoding: "cl100k_base"
```

## Global Tool Policy

**File:** `features/tools.yaml`

```yaml
tools:
  policy:
    auto: []
    confirm: []
    deny:
      - "Bash:sudo *"
      - "Bash:rm -rf *"
```

The global policy is a baseline merged additively with agent, mode, and task policies. See [Tools — Global Tool Policy]({{ site.baseurl }}/features/tools#global-tool-policy).

## Override Precedence

Each level merges non-zero values on top of the previous:

1. **Global defaults** — `features/*.yaml` (+ `ApplyDefaults()` for omitted fields)
2. **CLI flags** — e.g. `--max-steps` overrides `tools.max_steps`
3. **Agent frontmatter** — `features:` block (see [Agents]({{ site.baseurl }}/configuration/agents#feature-overrides))
4. **Mode frontmatter** — `features:` block (see [Modes]({{ site.baseurl }}/configuration/modes#feature-overrides))
5. **Task definition** — `features:` block (see [Tasks]({{ site.baseurl }}/commands/task))
6. **`--override` / `-O`** — dot-notation overrides (highest priority, see [Runtime Overrides]({{ site.baseurl }}/configuration#runtime-overrides))

Omitting a field preserves the value from the previous level.

{% endraw %}
