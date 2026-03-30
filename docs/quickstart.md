---
layout: default
title: Quick Start
nav_order: 3
---

# Quick Start

## First Session

Launch Aura to start an interactive session:

```sh
aura
```

Type a prompt and press `Enter`. Aura streams the response and executes tools as needed.

## One-Off Prompt

Run a single prompt non-interactively with `aura run`:

```sh
aura run "Write a Go function that reverses a string"
```

Pipe input from stdin:

```sh
echo "Explain this error" | aura run
```

Override the agent:

```sh
aura --agent high run "Summarize the changes in git diff"
```

## Agents

Agents define which model, provider, and system prompt to use. Switch with `Shift+Tab` or `/agent`.

Switch agents to change the model or provider (e.g., from local Ollama to cloud Anthropic). Switch modes to change tool availability (e.g., from read-only Ask to full-access Edit).

Edit or add agent files in `.aura/config/agents/`.

## Modes

Modes control which tools are available. Switch with `Tab` or `/mode`.

## Key Shortcuts

| Key             | Action                                       |
| --------------- | -------------------------------------------- |
| `Enter`         | Send message                                 |
| `Alt+Enter`     | Insert newline                               |
| `Ctrl+C`        | Copy selection / clear input / cancel / quit |
| `Esc`           | Cancel current streaming                     |
| `Tab`           | Cycle mode                                   |
| `Shift+Tab`     | Cycle agent                                  |
| `Ctrl+T`        | Toggle thinking visibility                   |
| `Ctrl+R`        | Toggle thinking on/off                       |
| `Ctrl+E`        | Cycle thinking level                         |
| `Ctrl+A`        | Toggle auto mode                             |
| `Ctrl+S`        | Toggle sandbox                               |
| `Ctrl+O`        | Open full tool output in pager               |
| `PgUp` / `PgDn` | Scroll chat history                          |

See [Keybindings]({{ site.baseurl }}/ui/keybindings) for the full list.

## Next Steps

- [Configuration]({{ site.baseurl }}/configuration/) — Customize agents, modes, providers, and features
- [Features]({{ site.baseurl }}/features/) — Tools, slash commands, embeddings, and more
- [Commands]({{ site.baseurl }}/commands/) — CLI subcommands and flags
- [UI]({{ site.baseurl }}/ui/) — TUI keybindings, status bar, and visual styles
- [Contributing]({{ site.baseurl }}/contributing/) — Architecture, extending, testing, and package organization
