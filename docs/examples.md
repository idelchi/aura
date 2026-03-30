---
layout: default
title: Examples
nav_order: 4
---

# Examples

Patterns and recipes for common workflows.

## Task Orchestration

See [Tasks]({{ site.baseurl }}/commands/task) for the full task file reference.

### Plan, build, and verify

```yaml
# .aura/config/tasks/build-app.yaml
build-app:
  agent: high
  timeout: 60m
  commands:
    - /mode plan
    - Read SPEC.md and generate a plan.
    - /until not todo_empty "You MUST call TodoCreate to create the plan"
    - /export logs/plan.log

    - /mode edit
    - /auto on
    - Execute the plan.

    - /until bash:"go build ./..." "Build is failing. Fix the errors."
    - /until bash:"golangci-lint run" "Linting is failing. Fix the errors."

    - /export logs/result.log
    - /stats
```

```sh
aura tasks run build-app --now
```

### Iterate over repositories

```yaml
test-all-repos:
  timeout: 30m
  agent: high
  mode: edit
  foreach:
    shell: "find ~/projects -maxdepth 1 -name 'go.mod' -exec dirname {} \\;"
    continue_on_error: true
    retries: 1
  commands:
    - |
      Run `go test ./...` in $[[ .Item ]].
      If any tests fail, investigate and report the failures.
  finally:
    - Summarize test results across all $[[ .Total ]] repositories.
```

## Condition Gates

See [Slash Commands]({{ site.baseurl }}/features/slash-commands#assert--conditional-actions) for the full condition list.

### /assert

```sh
aura run '/assert bash:"go build ./..." "Build passed!"'
aura run '/assert todo_done "/save"'
aura run '/assert context_above:80 "Context is filling up. Consider /compact."'
```

### /until

```sh
aura run '/until bash:"go test ./..." "Tests are failing. Fix them."'
aura run '/until --max 5 bash:"curl -sf http://localhost:8080/health" "Service is not healthy."'
```

## Bash Rewrite

`tools.bash.rewrite` wraps every Bash command. Template receives `{{ .Command }}` and all sprig functions.

### Token-optimized output with rtk

```yaml
tools:
  bash:
    rewrite: |
      if command -v rtk >/dev/null 2>&1 && REWRITTEN=$(rtk rewrite {{ .Command }} 2>/dev/null); then
        eval "${REWRITTEN}"
      else
        {{ .Command }}
      fi
```

### Activate a virtualenv

```yaml
tools:
  bash:
    rewrite: |
      source .venv/bin/activate && {{ .Command }}
```

### Run inside a container

```yaml
tools:
  bash:
    rewrite: |
      docker exec -w /app mycontainer sh -c '{{ .Command }}'
```

## Go Plugins

See [Plugins]({{ site.baseurl }}/features/plugins) for the full authoring guide.

### Lifecycle hook

```go
func AfterToolExecution(ctx context.Context, c sdk.AfterToolContext) (sdk.Result, error) {
    if c.Tool.Error != "" && c.Tool.Name == "Bash" {
        return sdk.Result{
            Message: "Bash command failed. Try a different approach.",
        }, nil
    }
    return sdk.Result{}, nil
}
```

### Custom tool

```go
func Schema() sdk.ToolSchema {
    return sdk.ToolSchema{
        Name:        "Greet",
        Description: "Say hello",
        Parameters: sdk.ToolParameters{
            Type:       "object",
            Properties: map[string]sdk.ToolProperty{
                "name": {Type: "string", Description: "Who to greet"},
            },
            Required: []string{"name"},
        },
    }
}

func Execute(ctx context.Context, sc sdk.Context, args map[string]any) (string, error) {
    return "Hello, " + args["name"].(string) + "!", nil
}
```

Install: `aura plugins add https://github.com/user/my-plugin`

## Config Inheritance

```yaml
---
inherit: [base, restricted]
model: qwen3:32b
mode: edit
---
```

## Template Variables

```sh
aura --set PROJECT=myapp --set ENV=staging run "Deploy $PROJECT to $ENV"
```

```yaml
deploy:
  vars:
    TARGET: staging # --set overrides these
  commands:
    - Deploy the application to $[[ .TARGET ]]
```

## Config Overrides

Override any feature or model setting from the command line:

```sh
# Limit tool iterations for a quick task
aura -O features.tools.max_steps=5 run "fix the typo"

# Low temperature for deterministic output
aura -O model.generation.temperature=0.1 run "translate this precisely"

# Multiple overrides
aura -O features.guardrail.mode=block -O model.context=200000 run "review this code"
```

## Prompt Templating

See [System Prompts]({{ site.baseurl }}/configuration/prompts) for all available template variables.

```
{% raw %}
{{- if .Hooks.Active }}
These hooks run after your tool calls:
{{ range .Hooks.Active -}}
- {{ .Name }}: {{ .Description }}
{{ end -}}
{{- end }}

{{- if .Model.Vision }}
You can analyze images. Use the Vision tool when the user provides screenshots.
{{- end }}

{{- if .Sandbox.Enabled }}
You are running in a sandboxed environment.
{{ .Sandbox.Display }}
{{- end }}
{% endraw %}
```

## Input Directives

See [Directives]({{ site.baseurl }}/features/directives) for full reference.

```
@File[config.yaml]     # Inline file contents
@Image[screenshot.png] # Attach image for vision models
@Path[~/projects]      # Resolve path with env expansion
@Bash[git log -5]      # Inline shell command output
```

## Skills

See [aura skills]({{ site.baseurl }}/commands/skills) for installation.

```markdown
---
name: commit
description: Create a well-formatted git commit
---

Review the staged changes with `git diff --cached`.
Write a commit message following conventional commits.
Run `git commit -m "<message>"`.
```
