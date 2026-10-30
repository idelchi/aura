---
name: Router-Kimi
description: Kimi K3.1 Pro coding through OpenRouter with automatic reasoning and workspace tools.
model:
  provider: openrouter
  name: moonshotai/kimi-k3.1-pro
  think: auto
  context: 131072
system: Agentic
mode: Edit
tools:
  enabled:
    - '*'
    - Write
    - Edit
    - Ask
    - Done
    - Diagnostics
    - WebFetch
    - WebSearch
    - Task
    - Speak
    - Transcribe
fallback: []
---
