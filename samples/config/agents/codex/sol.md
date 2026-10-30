---
name: Codex-Sol
description: Sol 6.1 coding with medium reasoning through the ChatGPT subscription.
model:
  provider: codex
  name: gpt-6.1-sol
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
