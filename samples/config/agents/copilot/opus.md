---
name: Copilot-Opus
description: Claude Opus 5.5 coding with medium reasoning through GitHub Copilot.
model:
  provider: copilot
  name: claude-opus-5.5
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
