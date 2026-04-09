---
layout: default
title: aura run
parent: Commands
nav_order: 1
---

# aura run

Execute prompts non-interactively.

## Syntax

```sh
aura run [prompts...]
```

## Description

Executes prompts through the assistant and exits. Multiple arguments are treated as separate turns — the assistant responds to each before processing the next. Prompts can also be piped via stdin.

Use `--timeout` to set a maximum execution time for all prompts. A timeout of 0 (the default) means no limit.

For file-based prompt sequences and scheduled execution, use [`aura tasks`](task.md) instead.

## Flags

| Flag        | Short | Default | Description                                            |
| ----------- | ----- | ------- | ------------------------------------------------------ |
| `--timeout` |       | `0`     | Maximum execution time for all prompts (0 = unlimited) |

Plus all [global flags]({{ site.baseurl }}/commands/#global-flags).

## Examples

```sh
# Single prompt
aura run "Write a Go function that parses CSV files"

# Multi-turn
aura run "/verbose true" "/mode plan" "Generate a plan for auth"

# Piped input
echo "Explain this error" | aura run

# With agent override
aura --agent high run "Summarize the changes in git diff"

# With timeout
aura run --timeout 2m "Summarize the codebase"

# Dry-run render (print what would be sent, then exit)
aura --dry=render run "test prompt"

# Dry-run noop (full pipeline, no LLM)
aura --dry=noop run "test prompt"
```

## Conditional Workflows

Use `/assert` and `/until` in YAML files to build multi-phase workflows with condition gates and retry loops.

```
/assert [not] <condition> "action1" ["action2" ...]
/until [--max N] [not] <condition> "action1" ["action2" ...]
```

`/assert` fires actions once when the condition is true. `/until` loops until the condition becomes true, executing actions each iteration when it is not yet met.

| Condition                                       | True when                                                                                               |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `todo_empty`                                    | No todo items and no summary                                                                            |
| `todo_done`                                     | All todo items completed, or list is empty                                                              |
| `todo_pending`                                  | At least one pending or in-progress item                                                                |
| `auto`                                          | Auto mode is enabled                                                                                    |
| `context_above:<N>`                             | Token usage >= N%                                                                                       |
| `context_below:<N>`                             | Token usage < N%                                                                                        |
| `history_gt:<N>` / `history_lt:<N>`             | Message count comparison                                                                                |
| `tool_errors_gt:<N>` / `tool_errors_lt:<N>`     | Tool error count comparison                                                                             |
| `tool_calls_gt:<N>` / `tool_calls_lt:<N>`       | Tool call count comparison                                                                              |
| `turns_gt:<N>` / `turns_lt:<N>`                 | LLM turn count comparison                                                                               |
| `tokens_total_gt:<N>` / `tokens_total_lt:<N>`   | Cumulative tokens (in+out) comparison                                                                   |
| `iteration_gt:<N>` / `iteration_lt:<N>`         | Current iteration comparison                                                                            |
| `compactions_gt:<N>` / `compactions_lt:<N>`     | Compaction count comparison                                                                             |
| `model_context_gt:<N>` / `model_context_lt:<N>` | Model max context comparison                                                                            |
| `model_params_gt:<N>` / `model_params_lt:<N>`   | Model parameters in billions (supports decimals)                                                        |
| `model_has:<cap>`                               | Model has capability (vision, tools, thinking, thinking_levels, embedding, reranking, context_override) |
| `model_is:<name>`                               | Model name matches (case-insensitive)                                                                   |
| `exists:<path>`                                 | File or directory exists (relative to CWD or absolute)                                                  |
| `bash:<cmd>`                                    | Shell command exits 0 (120s timeout)                                                                    |

Prefix any condition with `not` to negate it. Join conditions with `and` for composition. Actions must be quoted.

See [/assert reference]({{ site.baseurl }}/features/slash-commands#assert--conditional-actions) and [/until reference]({{ site.baseurl }}/features/slash-commands#until--looping-conditional) for full details.

### Plan-then-execute

```yaml
- /mode plan
- Design a REST API for user management
- /until not todo_empty "You MUST call TodoCreate to create the plan"
- /mode edit
- Execute the plan
```

### Retry until build passes

```yaml
- /until bash:"go build ./..." "Build is failing. Fix the errors."
- /assert bash:"go build ./..." "Build passed!"
```

### Guarded auto mode

```yaml
- /auto
- /done
- Implement the feature described in SPEC.md
- /assert not todo_done "There are still incomplete items. Finish them all."
- /assert todo_done "/save"
```
