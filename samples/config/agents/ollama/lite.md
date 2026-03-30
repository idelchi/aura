---
name: lite
description: Lightweight agent for simple tasks

model:
  provider: ollama
  name: granite4:micro-h
  context: 16384

tools:
  disabled:
    - Ask

system: Lite
mode: Lite
agentsmd: none

features:
  compaction:
    prompt: Compaction
---
