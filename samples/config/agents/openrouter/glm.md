---
name: GLM

description: GLM 5.3 coding through OpenRouter with automatic reasoning and workspace tools.
model:
  provider: openrouter
  name: z-ai/glm-5.3
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
