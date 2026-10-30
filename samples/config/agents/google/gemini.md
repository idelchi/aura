---
name: Gemini

description: Gemini 3.8 Flash coding through Google with automatic reasoning and workspace tools.
model:
  provider: google
  name: gemini-3.8-flash
  think: auto

  context: 131072
thinking: ""


system: Agentic
mode: Edit
tools:
  enabled: ["*"]
  disabled: []
---
You can analyze images and documents.
