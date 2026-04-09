---
name: Compaction
description: Agent for summarizing conversation context during handoff to another LLM.

model:
  provider: ollama
  name: gpt-oss:20b
  think: medium
  context: 32768

tools:
  disabled: ["*"]

hide: true
agentsmd: none
system: Compaction
---
