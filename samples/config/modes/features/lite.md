---
name: Lite

hide: true

features:
  sandbox:
    extra:
      rw:
        - .
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
