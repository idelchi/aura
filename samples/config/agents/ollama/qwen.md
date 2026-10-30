---
default: true
description: Local Ollama coding preset; configure a model installed on your server.
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
  context: 32768
  generation:
    seed: 42
    temperature: 0.6
    top_k: 20
    top_p: 0.95
  name: qwen3.5:9b-q8_0
  provider: ollama
  think: true
name: Qwen
tools:
  disabled: []
  enabled:
  - '*'
  - Write
  - Edit
---
