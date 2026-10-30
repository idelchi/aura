---
name: Qwen
description: Default Qwen 3.5 9B coding through local Ollama with thinking enabled and workspace tools.
inherit:
  - High
default: true
model:
  provider: ollama
  name: qwen3.5:9b-q8_0
  think: true
  context: 32768
  generation:
    seed: 42
    temperature: 0.6
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
