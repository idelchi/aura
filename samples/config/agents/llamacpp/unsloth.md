---
name: Unsloth

model:
  provider: llamacpp
  name: unsloth/Qwen3-4B-Instruct-2507-GGUF:Q8_0

tools:
  disabled:
    - "*"

system: Lite
mode: Ask

features:
  compaction:
    prompt: Compaction
---
