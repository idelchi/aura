---
name: OpenAI-Luna
description: Luna 6 coding with medium reasoning through the OpenAI API.
model:
  provider: openai
  name: gpt-6-luna
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
