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
You have access to the following currently loaded tools:

tools:
{{ range .Tools.Eager }}- {{ . }}
{{ end }}
{{ if not .Tools.Deferred }}No other tools exist.{{ end }}
{{ else -}}
You have NO tools available. Do not attempt to make tool calls.
{{ end -}}
