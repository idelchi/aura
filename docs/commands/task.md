---
layout: default
title: aura tasks
parent: Commands
nav_order: 11
---

# aura tasks

Manage and run scheduled tasks.

## Syntax

```sh
aura tasks [--files ...] run [names...] [--now] [--concurrency N] [--prepend ...] [--append ...] [--start N] [--timeout D]
```

## Description

Tasks are named sets of commands defined in `.aura/config/tasks/*.yaml`. Each task has a list of commands (slash commands, prompts, and `!`-prefixed shell commands) that execute sequentially through the assistant. Tasks can optionally have a cron-like schedule for automatic execution via the daemon. Use [`aura show tasks`]({{ site.baseurl }}/commands/show) to list and inspect tasks.

## Flags

| Flag            | Default | Description                                                     |
| --------------- | ------- | --------------------------------------------------------------- |
| `--files`       |         | Additional task file globs to load (repeatable, `**` supported) |
| `--now`         | `false` | Run tasks immediately instead of on schedule                    |
| `--concurrency` | `1`     | Maximum number of tasks to run in parallel                      |
| `--prepend`     |         | Commands to insert before the task's command list (repeatable)  |
| `--append`      |         | Commands to append after the task's command list (repeatable)   |
| `--start`       | `0`     | Skip first N commands (or N items for foreach tasks)            |
| `--timeout`     | `0`     | Override task timeout (0 = use task's configured timeout)       |

Root flags `--agent` and `--mode` take precedence over task YAML when explicitly set.

Plus all [global flags]({{ site.baseurl }}/commands/#global-flags).

## Task File Format

Tasks are YAML files in `.aura/config/tasks/`. Each file is a map of task names to definitions:

```yaml
daily-review:
  description: Summarize yesterday's git activity
  schedule: "cron: 0 9 * * 1-5"
  timeout: 5m
  agent: high
  mode: Ask
  pre:
    - git pull --rebase
  commands:
    - >
      Review git commits from the last 24 hours.
      Summarize changes and flag potential issues.
  post:
    - echo "Review complete"

reindex:
  schedule: "daily: 02:00"
  timeout: 10m
  agent: high
  tools:
    enabled: ["Query"]
  commands:
    - "/query"
```

### Fields

| Field          | Type     | Default      | Description                                                                                                                         |
| -------------- | -------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| `description`  | string   | `""`         | Human-readable summary                                                                                                              |
| `schedule`     | string   | `""`         | Schedule expression. Empty = manual-only                                                                                            |
| `timeout`      | duration | `5m`         | Max wall-clock time per execution                                                                                                   |
| `agent`        | string   | `""`         | Agent to activate before running commands                                                                                           |
| `mode`         | string   | `""`         | Mode to activate before running commands                                                                                            |
| `session`      | string   | `""`         | Session name to resume                                                                                                              |
| `workdir`      | string   | `""`         | Working directory for task execution                                                                                                |
| `disabled`     | bool     | `false`      | Skip this task without removing its definition                                                                                      |
| `tools`        | object   | `{}`         | Tool filter with `enabled`/`disabled` glob patterns                                                                                 |
| `features`     | object   | `{}`         | Feature overrides merged on top of the agent's effective features                                                                   |
| `vars`         | map      | `nil`        | Task-scoped template variables                                                                                                      |
| `env`          | map      | `nil`        | Task-scoped environment variables                                                                                                   |
| `env_file`     | []string | `[]`         | Dotenv files to load (relative to config home)                                                                                      |
| `inherit`      | list     | `[]`         | Inherit from parent task(s)                                                                                                         |
| `pre`          | []string | `[]`         | Shell commands to run before the assistant                                                                                          |
| `commands`     | []string | **required** | Command sequence: prompts, `/slash` commands, or `!shell` commands                                                                  |
| `post`         | []string | `[]`         | Shell commands to run after the assistant                                                                                           |
| `foreach`      | object   | `nil`        | Iteration source — `file:` or `shell:`                                                                                              |
| `finally`      | []string | `[]`         | Commands to run once after the foreach loop (requires `foreach`)                                                                    |
| `on_max_steps` | []string | `[]`         | Shell commands executed when the task hits its `max_steps` limit. Runs outside the LLM loop — useful for sending alerts or cleanup. |

### Schedule Syntax

| Prefix     | Example                      | Description                       |
| ---------- | ---------------------------- | --------------------------------- |
| `cron:`    | `cron: 0 9 * * 1-5`          | Standard 5-field crontab          |
| `every:`   | `every: 30m`                 | Fixed interval (Go duration)      |
| `daily:`   | `daily: 09:00,17:00`         | Every day at specified times      |
| `weekly:`  | `weekly: mon,wed,fri 09:00`  | Specific weekdays + time          |
| `monthly:` | `monthly: 1,15 09:00`        | Specific days of month + time     |
| `once:`    | `once: 2026-03-01T09:00:00Z` | Single future execution (RFC3339) |
| `once:`    | `once: startup`              | Run once when daemon starts       |

### Template Variables

{% raw %}

Task files support two delimiter systems:

- **Load-time `{{ }}`** — expanded before YAML parsing. Sprig functions, env vars, `--set` variables, and control flow.
- **Runtime `$[[ ]]`** — expanded per-command at execution time. Used for iteration variables and execution context.

| Variable     | Scope        | Description                        |
| ------------ | ------------ | ---------------------------------- |
| `.Workdir`   | All          | Effective working directory        |
| `.LaunchDir` | All          | Directory where aura was invoked   |
| `.Date`      | All          | Trigger time (filesystem-friendly) |
| `.Item`      | Foreach only | Current line content               |
| `.Index`     | Foreach only | Zero-based iteration index         |
| `.Total`     | Foreach only | Total number of items              |

`$[[ ]]` avoids collision with bash `[[ ]]` syntax in `!`-prefixed shell commands.

Task-scoped variables via `vars:` are available in both template systems. `--set` flags override them:

```yaml
challenge:
  vars:
    MODEL: gpt-oss:20b
  commands:
    - "What model are you? You should be $[[ .MODEL ]]"
```

```sh
aura --set MODEL=qwen3:14b tasks run challenge
```

{% endraw %}

## Session Continuity

Tasks with a `session:` field resume a named session before executing and auto-save after. Without `session:`, each run starts fresh.

## Tool Filtering

Tasks restrict tools using `enabled`/`disabled` glob patterns. The filtering chain is: AllTools → agent → mode → task — each level can only further restrict, never expand.

```yaml
read-only-review:
  agent: high
  tools:
    disabled: [Bash, Patch, Mkdir]
  commands:
    - "Review the codebase for potential issues"
```

## Feature Overrides

Override chain: **global → CLI flags → agent → mode → task**.

```yaml
heavy-refactor:
  agent: high
  features:
    tools:
      max_steps: 200
    compaction:
      threshold: 90
  commands:
    - "Refactor the auth module"
```

Available feature keys: `compaction`, `title`, `thinking`, `vision`, `embeddings`, `tools`, `stt`, `tts`, `sandbox`, `subagent`, `plugins`, `mcp`, `estimation`, `guardrail`. See [Features]({{ site.baseurl }}/configuration/features).

## Environment

```yaml
deploy:
  env:
    OUTPUT_DIR: "/tmp/output"
  env_file:
    - secrets.env
  commands:
    - "Deploy to the target environment"
```

**Precedence:** `env:` > `env_file:` > process env.

## Shell Commands

Prefix a command entry with `!` to run it as a shell command instead of sending it to the LLM. Multiline scripts: put `!` alone on the first line, script body below. Errors abort the task; use `|| true` to suppress. Shell output goes directly to stdout/stderr — not visible to the LLM.

## Pre/Post Hooks

`pre:` runs before session/agent/mode setup; any failure aborts the task. `post:` runs after all commands complete. See [Hooks]({{ site.baseurl }}/configuration/hooks).

## Foreach

Iterate over lines from a file or shell command. Each iteration expands runtime template variables.

{% raw %}

```yaml
logs:
  description: Analyze logs of all containers
  timeout: 1h
  agent: logs
  pre:
    - aura tools --mcp-servers portainer mcp__portainer__containers "{}" --raw --headless > /tmp/containers
  foreach:
    file: /tmp/containers
    continue_on_error: true
    retries: 1
  commands:
    - /new
    - |
      Call mcp__portainer__logs with: { "container": "$[[ base .Item ]]", "environment": "$[[ dir .Item ]]" }
      Analyze the logs and report critical issues.
  finally:
    - "Read all findings and summarize the most critical issues."
```

{% endraw %}

| Field                | Description                                                 |
| -------------------- | ----------------------------------------------------------- |
| `file:`              | Read lines from a file (one per line, empty lines filtered) |
| `shell:`             | Run a command and read lines from stdout                    |
| `continue_on_error:` | Log per-item errors and continue instead of aborting        |
| `retries:`           | Additional attempts per failed item (0 = no retry)          |

`finally:` runs once after all iterations through the assistant.

## Concurrency

- `--concurrency N` caps total parallel task executions (default 1 = sequential).
- In scheduler mode, if a task is still running when its next trigger fires, the trigger is skipped.

## Examples

```sh
# Start the scheduler for all scheduled tasks
aura tasks run

# Run specific tasks immediately
aura tasks run --now daily-review reindex

# Run with command override
aura tasks run --now daily-review --prepend "/mode Ask"

# Run with parallel execution
aura tasks run --concurrency 3
```
