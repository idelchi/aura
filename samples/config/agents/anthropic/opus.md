---
name: Claude-Opus
description: Claude Opus 5.5 for repository edits and debugging with medium reasoning through Anthropic.
model:
  provider: anthropic
  name: claude-opus-5-5
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
