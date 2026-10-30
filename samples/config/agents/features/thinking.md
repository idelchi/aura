---
name: Thinking
description: Condense reasoning blocks into short summaries for the thinking display.
model:
  provider: ollama
  name: qwen3.5:9b-q8_0
  think: false
  context: 32768
system: Thinking
tools:
  disabled:
    - '*'
hide: true
agentsmd: none
---
