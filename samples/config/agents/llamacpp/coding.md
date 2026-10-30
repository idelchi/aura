---
name: LlamaCpp
description: Qwen Flash-Next coding on a local llama.cpp worker with medium reasoning and workspace tools.
inherit:
  - High
model:
  provider: llamacpp
  name: qwen3.8:flash-next
  think: medium
  context: 131072
  generation:
    seed: 42
    temperature: 1
    top_k: 20
    top_p: 0.95
mode: Edit
tools:
  disabled: []
  enabled:
    - '*'
    - Write
    - Edit
features:
  compaction:
    prune:
      mode: compaction
  estimation:
    method: rough+tiktoken
---
