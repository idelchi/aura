---
name: Explore
description: Read-only repository investigation using Qwen 3.5 9B through local Ollama.
model:
  provider: ollama
  name: qwen3.5:9b-q8_0
  think: true
  context: 32768
  generation:
    seed: 42
    temperature: 1
    top_k: 20
    top_p: 0.95
system: Agentic
mode: Ask
tools:
  enabled:
    - Read
    - Glob
    - Rg
    - Bash
    - Skill
    - WebFetch
    - WebSearch
hide: true
subagent: true
fallback: []
---
