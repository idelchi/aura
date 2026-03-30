---
layout: default
title: Commands
nav_order: 6
has_children: true
---

# Commands

Aura's CLI is built with [urfave/cli v3](https://github.com/urfave/cli). The root `aura` command launches the interactive assistant. Subcommands provide non-interactive utilities.

| Command           | Description                                                                                                   |
| ----------------- | ------------------------------------------------------------------------------------------------------------- |
| `aura`            | Interactive assistant (default)                                                                               |
| `aura run`        | Non-interactive prompt execution (inline or stdin)                                                            |
| `aura models`     | List available models                                                                                         |
| `aura tools`      | List, inspect, or execute tools                                                                               |
| `aura query`      | Embedding-based codebase search                                                                               |
| `aura vision`     | Analyze an image or PDF via a vision-capable LLM                                                              |
| `aura transcribe` | Transcribe an audio file to text                                                                              |
| `aura speak`      | Convert text to speech audio                                                                                  |
| `aura tokens`     | Count tokens in a file or stdin                                                                               |
| `aura mcp`        | List configured MCP servers and their tools                                                                   |
| `aura show`       | List and inspect config entities (agents, modes, prompts, providers, hooks, features, plugins, skills, tasks) |
| `aura tasks`      | Manage and run scheduled tasks                                                                                |
| `aura plugins`    | Manage plugins (add, remove, update)                                                                          |
| `aura skills`     | Manage skills (add, remove, update)                                                                           |
| `aura login`      | Authenticate with an OAuth provider (device code flow)                                                        |
| `aura cache`      | Manage the cache (clean)                                                                                      |
| `aura web`        | Start browser-based UI                                                                                        |
| `aura init`       | Scaffold default configuration                                                                                |

## Global Flags

These flags are available on all commands. They must appear **before** the subcommand name:

```sh
# Correct
aura --agent high run "Hello"

# Wrong — flag after subcommand is not recognized
aura run --agent high "Hello"
```

| Flag                | Short | Default       | Description                                                                                                                                           |
| ------------------- | ----- | ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--config`          |       | `.aura`       | Configuration directories to merge (repeatable, left-to-right, last wins)                                                                             |
| `--version`         | `-v`  |               | Print version and exit                                                                                                                                |
| `--provider`        |       |               | LLM provider to use                                                                                                                                   |
| `--agent`           |       | _(auto)_      | Agent to use (resolves via `default: true` → first visible)                                                                                           |
| `--model`           | `-m`  |               | Model to use                                                                                                                                          |
| `--env-file`        | `-e`  | `secrets.env` | Environment files to load (repeatable)                                                                                                                |
| `--show`            | `-s`  | `false`       | Dump the full resolved YAML config and exit (no session started)                                                                                      |
| `--simple`          |       | `false`       | Use simple readline-based TUI                                                                                                                         |
| `--auto`            |       | `false`       | Enable auto mode                                                                                                                                      |
| `--mode`            |       |               | Starting mode (ask, edit, plan)                                                                                                                       |
| `--system`          |       |               | System prompt name (overrides agent config; matches `name:` from prompt files in `config/prompts/`)                                                   |
| `--think`           |       |               | Thinking level (off, on, low, medium, high)                                                                                                           |
| `--include-tools`   |       |               | Glob patterns for tools to include (e.g. `Read,Glob,Rg`)                                                                                              |
| `--exclude-tools`   |       |               | Glob patterns for tools to exclude (e.g. `Bash,Patch`)                                                                                                |
| `--include-mcps`    |       |               | Glob patterns for MCP servers to connect (e.g. `context7,git*`)                                                                                       |
| `--exclude-mcps`    |       |               | Glob patterns for MCP servers to skip (e.g. `portainer`)                                                                                              |
| `--max-steps`       |       | `0`           | Maximum tool-use iterations (overrides config)                                                                                                        |
| `--token-budget`    |       | `0`           | Cumulative token limit (overrides config)                                                                                                             |
| `--workdir`         | `-w`  |               | Working directory for tool execution and path resolution                                                                                              |
| `--resume`          |       |               | Resume a saved session by ID prefix                                                                                                                   |
| `--continue`        | `-c`  | `false`       | Resume the most recent session                                                                                                                        |
| `--output`          | `-o`  |               | Mirror all output to a file                                                                                                                           |
| `--without-plugins` |       | `false`       | Disable Go plugins                                                                                                                                    |
| `--unsafe-plugins`  |       | `false`       | Allow plugins to use os/exec and other restricted imports                                                                                             |
| `--debug`           |       | `false`       | Enable debug logging                                                                                                                                  |
| `--print`           |       | `false`       | Print a one-line config summary and exit                                                                                                              |
| `--print-env`       |       | `false`       | Print resolved settings as `AURA_*` environment variables and exit                                                                                    |
| `--experiments`     |       | `false`       | Enable experimental features                                                                                                                          |
| `--home`            |       | `~/.aura`     | Global config home (~/.aura); set to empty to disable                                                                                                 |
| `--providers`       |       |               | Limit to these providers (filters model listings and discards non-matching agents)                                                                    |
| `--set`             |       |               | Set template variables (KEY=value, repeatable)                                                                                                        |
| `--override`        | `-O`  |               | Override config values (dot-notation, repeatable). Example: `-O features.tools.max_steps=10`                                                          |
| `--dry`             |       |               | Dry-run mode: `render` prints resolved agent, prompt, and input then exits; `noop` runs the full pipeline with a no-op provider that returns `[noop]` |
| `--no-cache`        |       | `false`       | Bypass cached data and re-fetch from source                                                                                                           |

> **Note**: All root flags can be set via environment variables with the `AURA_` prefix (e.g. `AURA_AGENT`, `AURA_MODEL`, `AURA_DEBUG`). Use `aura --print-env` to see all resolved settings as environment variables.
