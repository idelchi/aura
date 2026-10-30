---
name: Codex-Astra
description: Astra 6 coding with medium reasoning through the ChatGPT subscription.
model:
  provider: codex
  name: gpt-6-astra
  think: medium
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
