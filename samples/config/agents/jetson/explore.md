---
name: explore
description: Lightweight agent for exploration style tasks
subagent: true

model:
  provider: anthropic
  name: claude-haiku-4-5-20251001

tools:
  enabled:
    - Bash
    - Glob
    - Ls
    - Read
    - Rg

system: Lite
agentsmd: none

features:
  compaction:
    prompt: Compaction
---

{{ if .Tools.Eager -}}
You have access to the following:

tools:
{{ range .Tools.Eager }}- {{ . }}
{{ end }}
No other tools exist.
{{ else -}}
You have NO tools available. Do not attempt to make tool calls.
{{ end -}}
