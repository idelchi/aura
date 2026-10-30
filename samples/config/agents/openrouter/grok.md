---
name: Grok

description: Grok 4.7 coding through OpenRouter with automatic reasoning and workspace tools.
model:
  provider: openrouter
  name: x-ai/grok-4.7
  think: auto

  context: 131072

system: Agentic
mode: Edit

tools:
  enabled: ["*"]
features:
  compaction:
    prompt: Compaction
---
