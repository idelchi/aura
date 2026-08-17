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
You have access to the following currently loaded tools:

tools:
{{ range .Tools.Eager }}- {{ . }}
{{ end }}
{{ if not .Tools.Deferred }}No other tools exist.{{ end }}
{{ else -}}
You have NO tools available. Do not attempt to make tool calls.
{{ end -}}
