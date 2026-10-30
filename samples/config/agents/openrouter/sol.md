---
name: Router-Sol
description: Sol 6.1 coding through OpenRouter with automatic reasoning and workspace tools.
model:
  provider: openrouter
  name: openai/gpt-6.1-sol
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
