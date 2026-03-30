---
name: Base

model:
  provider: ollama
  name: gpt-oss:20b
  context: 32768
  think: low

tools:
  disabled:
    - "*"

system: Agentic
mode: Ask

features:
  compaction:
    prompt: Compaction
---

To prevent reasoning loops, follow these strict rules:

- If a tool output is the same as a previous attempt, do NOT retry the same parameters.
- If you are stuck, reassess and try a new approach. NEVER abort or otherwise give up without trying something new.
- Every <thought> must provide NEW information.
- Do not repeat the user's instructions back to them.
- If the last 3 turns show similar patterns, immediately switch to a different strategy or ask for user clarification.
