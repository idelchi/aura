---
layout: default
title: aura show
parent: Commands
nav_order: 18
---

# aura show

List and inspect configuration entities loaded from disk.

## Syntax

```sh
aura show                              # Summary of entity counts
aura show <entity>                     # List all entities of that type
aura show <entity> [--filter key=val]  # Filter by YAML key path
aura show <entity> <name>              # Detail view of one entity
```

## Entities

| Subcommand  | Description                                                       |
| ----------- | ----------------------------------------------------------------- |
| `agents`    | Agent definitions (model, mode, prompt, tool filters)             |
| `modes`     | Mode definitions (tool availability, features)                    |
| `prompts`   | System prompts                                                    |
| `providers` | Provider configurations                                           |
| `hooks`     | Shell hooks (pre/post tool execution)                             |
| `features`  | Feature configuration (compaction, thinking, tools, vision, etc.) |
| `plugins`   | Go plugins                                                        |
| `skills`    | LLM-invocable skills                                              |
| `tasks`     | Scheduled task definitions                                        |

## Flags

| Flag       | Short | Description                                                                        |
| ---------- | ----- | ---------------------------------------------------------------------------------- |
| `--filter` | `-f`  | Filter by YAML key path (key=value, repeatable, dot notation, wildcards supported) |

Plus all [global flags]({{ site.baseurl }}/commands/#global-flags).

## Filtering

The `--filter` flag accepts YAML key paths using dot notation for nested fields. Values support `*` wildcards and are matched case-insensitively.

```sh
# Filter agents by mode
aura show agents --filter mode=Edit

# Filter agents by model provider
aura show agents --filter model.provider=anthropic

# Filter providers by type
aura show providers --filter type=ollama

# Filter hooks by event timing
aura show hooks --filter event=post

# Wildcard matching
aura show agents --filter model.name=gpt*
aura show providers --filter url=*garfield*
aura show plugins --filter description=*tool*

# Combine multiple filters (all must match)
aura show agents --filter mode=Edit --filter model.provider=ollama
```

Key paths follow the YAML structure of the entity. Use `aura show <entity> <name>` to see the available fields and discover key paths.

## Examples

```sh
# Show entity counts
aura show

# List all agents
aura show agents

# Detail view of a specific agent
aura show agents High

# List all providers
aura show providers

# List only ollama providers
aura show providers --filter type=ollama

# Show full feature configuration
aura show features
```

## Migration

`aura show` replaces the following commands:

| Old command                | New command                |
| -------------------------- | -------------------------- |
| `aura plugins list`        | `aura show plugins`        |
| `aura plugins show <name>` | `aura show plugins <name>` |
| `aura skills list`         | `aura show skills`         |
| `aura skills show <name>`  | `aura show skills <name>`  |
| `aura tasks list`          | `aura show tasks`          |
| `aura tasks show <name>`   | `aura show tasks <name>`   |

The management operations (`add`, `remove`, `update`, `run`) remain under their original parent commands (`aura plugins`, `aura skills`, `aura tasks`).
