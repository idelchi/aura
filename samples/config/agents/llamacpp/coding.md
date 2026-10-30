---
description: Local llama.cpp coding; model name must match the alias served by your
  worker.
features:
  compaction:
    prune:
      mode: compaction
  estimation:
    method: rough+tiktoken
inherit:
- High
mode: Edit
model:
  context: 131072
  generation:
    seed: 42
    temperature: 1
    top_k: 20
    top_p: 0.95
  name: qwen3.8:flash-next
  provider: llamacpp
  think: medium
name: LlamaCpp
tools:
  disabled: []
  enabled:
  - '*'
  - Write
  - Edit
---
