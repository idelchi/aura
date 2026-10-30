---
name: Compaction
description: Summarize conversation history while retaining task decisions, tool evidence and unfinished work.
model:
  provider: ollama
  name: qwen3.5:9b-q8_0
  think: true
  context: 32768
system: Compaction
tools:
  disabled:
    - '*'
hide: true
agentsmd: none
---
