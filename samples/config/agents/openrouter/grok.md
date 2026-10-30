---
name: grok

model:
  provider: openrouter
  name: x-ai/grok-4.7
  think: auto

  context: 131072
tools:
  enabled: ["*"]

system: Agentic
mode: Edit

features:
  compaction:
    prompt: Compaction
---
