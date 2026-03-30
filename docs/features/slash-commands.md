---
layout: default
title: Slash Commands
parent: Features
nav_order: 2
---

# Slash Commands

Aura includes built-in slash commands for runtime control, plus support for user-defined custom commands. Run `/help` to see them grouped in the TUI picker, or `/help <command>` for details on a specific command.

## Built-in Commands

### Agent

| Command    | Aliases   |
| ---------- | --------- |
| `/agent`   |           |
| `/mode`    |           |
| `/model`   |           |
| `/think`   | `/effort` |
| `/verbose` | `/v`      |

### Session

| Command   | Aliases |
| --------- | ------- |
| `/clear`  | `/new`  |
| `/export` |         |
| `/fork`   |         |
| `/name`   |         |
| `/resume` | `/load` |
| `/save`   |         |

### Tools

| Command    | Aliases     |
| ---------- | ----------- |
| `/mcp`     |             |
| `/plugins` |             |
| `/policy`  |             |
| `/sandbox` | `/landlock` |
| `/skills`  |             |
| `/tools`   |             |

### Context

| Command    | Aliases    |
| ---------- | ---------- |
| `/compact` |            |
| `/ctx`     | `/context` |
| `/drop`    | `/remove`  |
| `/insert`  |            |
| `/undo`    | `/rewind`  |
| `/window`  |            |

### Execution

| Command   | Aliases |
| --------- | ------- |
| `/assert` |         |
| `/auto`   |         |
| `/done`   |         |
| `/replay` |         |
| `/until`  |         |

### Config

| Command       | Aliases |
| ------------- | ------- |
| `/hooks`      |         |
| `/info`       |         |
| `/readbefore` | `/rb`   |
| `/reload`     |         |
| `/set`        |         |
| `/stats`      |         |

### Todo

| Command | Aliases |
| ------- | ------- |
| `/todo` |         |

### Search

| Command  | Aliases |
| -------- | ------- |
| `/query` |         |

### System

| Command | Aliases  |
| ------- | -------- |
| `/exit` | `/quit`  |
| `/help` |          |

---

## /assert — Conditional Actions

Evaluates a condition and executes quoted actions if true. If the condition is not met, does nothing.

### Syntax

```
/assert [not] <condition> "action1" ["action2" ...]
```

### Conditions

| Condition              | True when                                                                                               |
| ---------------------- | ------------------------------------------------------------------------------------------------------- |
| `todo_empty`           | No todo items and no summary (blank slate)                                                              |
| `todo_done`            | All todo items completed, or list is empty                                                              |
| `todo_pending`         | At least one pending or in-progress todo item                                                           |
| `auto`                 | Auto mode is enabled                                                                                    |
| `context_above:<N>`    | Token usage >= N%                                                                                       |
| `context_below:<N>`    | Token usage < N%                                                                                        |
| `history_gt:<N>`       | Message count > N                                                                                       |
| `history_lt:<N>`       | Message count < N                                                                                       |
| `tool_errors_gt:<N>`   | Tool errors > N                                                                                         |
| `tool_errors_lt:<N>`   | Tool errors < N                                                                                         |
| `tool_calls_gt:<N>`    | Tool calls > N                                                                                          |
| `tool_calls_lt:<N>`    | Tool calls < N                                                                                          |
| `turns_gt:<N>`         | LLM turns > N                                                                                           |
| `turns_lt:<N>`         | LLM turns < N                                                                                           |
| `compactions_gt:<N>`   | Compactions > N                                                                                         |
| `compactions_lt:<N>`   | Compactions < N                                                                                         |
| `iteration_gt:<N>`     | Current iteration > N                                                                                   |
| `iteration_lt:<N>`     | Current iteration < N                                                                                   |
| `tokens_total_gt:<N>`  | Cumulative tokens (in+out) > N                                                                          |
| `tokens_total_lt:<N>`  | Cumulative tokens (in+out) < N                                                                          |
| `model_context_gt:<N>` | Model max context > N tokens                                                                            |
| `model_context_lt:<N>` | Model max context < N tokens                                                                            |
| `model_has:<cap>`      | Model has capability (vision, tools, thinking, thinking_levels, embedding, reranking, context_override) |
| `model_params_gt:<N>`  | Model parameters > N billion (supports decimals like 0.5)                                               |
| `model_params_lt:<N>`  | Model parameters < N billion                                                                            |
| `model_is:<name>`      | Model name matches (case-insensitive)                                                                   |
| `exists:<path>`        | File or directory exists (relative to CWD or absolute)                                                  |
| `bash:<cmd>`           | Shell command exits 0 (120s timeout)                                                                    |

Prefix any condition with `not` to negate it. Use `and` for composition:

```
/assert "history_gt:5 and history_lt:10" "You're in the sweet spot."
/assert "auto and context_above:70" "/compact"
```

### Examples

```yaml
- /mode plan
- Plan the refactor
- /assert todo_empty "You forgot to create a todo list. Call TodoCreate now."
- /assert not todo_empty "/mode edit" "Execute the plan"
- /assert todo_pending "/mode edit" "/think high" "Execute all remaining tasks"
```

---

## /until — Looping Conditional

Loops until a condition becomes true. The condition is the **exit** condition. Same syntax as `/assert`; `--max N` caps iterations.

```
/until [--max N] [not] <condition> "action1" ["action2" ...]
```

```yaml
- /until not todo_empty "You MUST call TodoCreate to create the plan"
```

---

## /mcp — MCP Server Management

```
/mcp                      # List all MCP servers and their tools
/mcp reconnect            # Reconnect all enabled but unconnected servers
/mcp reconnect <server>   # Reconnect a specific server by name
```

---

## Custom Slash Commands

Markdown files with YAML frontmatter in `.aura/config/commands/`:

```markdown
---
name: review
description: "Code review the current changes"
hints:
  - "file path"
  - "focus area"
---

Review the following code changes with focus on:

- Correctness
- Edge cases
- Go idioms

Target: $1
Focus: $2
Full arguments: $ARGUMENTS
```

`$1`, `$2`, ... are positional arguments; `$ARGUMENTS` is the full string. The rendered body is sent to the LLM as a user message. Conflicts with built-in commands are detected at startup.

## Plugin-Defined Commands

Plugins can register slash commands by exporting `Command()` and `ExecuteCommand()`. Priority order: built-in > custom (Markdown) > plugin. Each plugin may define at most one command. See [Plugin Command Functions]({{ site.baseurl }}/features/plugins#command-functions) for details.

---

## Auto Mode

Auto mode enables continuous execution without requiring user confirmation between iterations. The assistant loop continues automatically as long as there are pending or in-progress todo items, or the LLM is making tool calls. It stops when all todos are completed, the LLM calls the **Done** tool, `max_steps` is reached, `token_budget` is exhausted, or the user cancels with `Ctrl+C`.

### Todo State

Auto mode is driven by todo state. Injectors such as `todo-reminder` and `todo-not-finished` fire during the loop to keep the LLM on track — see [Message Injection (Injectors)]({{ site.baseurl }}/features/hooks#message-injection-injectors).

### /auto Command

```
/auto          # Toggle auto mode on/off
Ctrl+A         # Keyboard shortcut to toggle
--auto         # CLI flag to start with auto mode enabled
```

---

## /replay — Replay a YAML Command File

Loads a YAML file and executes each item in sequence as if typed at the prompt. Items can be slash commands, messages, or any valid input. Recursive `/replay` calls are silently skipped.

### Syntax

```
/replay <file> [start-index]
```

### Example

```yaml
# workflow.yaml
- /mode plan
- Plan the refactor
- /assert not todo_empty "/mode edit" "Execute the plan"
```

```
/replay workflow.yaml
/replay workflow.yaml 2   # start from index 2
```
