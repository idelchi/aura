---
description: Read-only project investigation on the same local worker as Flash.
fallback: []
hide: true
mode: Ask
model:
  context: 32768
  generation:
    seed: 42
    temperature: 1
    top_k: 20
    top_p: 0.95
  name: qwen3.5:9b-q8_0
  provider: ollama
  think: true
name: Explore
subagent: true
system: Agentic
tools:
  enabled:
  - Read
  - Glob
  - Rg
  - Bash
  - Skill
  - WebFetch
  - WebSearch
---
