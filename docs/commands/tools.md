---
layout: default
title: aura tools
parent: Commands
nav_order: 3
---

# aura tools

List, inspect, or execute tools.

## Syntax

```sh
aura tools [name] [args...]
```

## Description

Three modes of operation:

1. **List all tools** — `aura tools` — shows all available tools with descriptions
2. **Inspect a tool** — `aura tools Read` — shows tool schema, usage, and examples
3. **Execute a tool** — `aura tools Read path=main.go` — runs the tool with arguments

Arguments can be passed as **key=value pairs** or as **JSON**. The format is auto-detected: if the argument starts with `{`, it's parsed as JSON; otherwise as key=value pairs. Use `--raw` to force JSON parsing.

## Flags

| Flag            | Short | Default | Description                                              |
| --------------- | ----- | ------- | -------------------------------------------------------- |
| `--json`        |       | `false` | Output as structured JSON                                |
| `--raw`         |       | `false` | Force JSON parsing of arguments                          |
| `--ro`          | `-R`  | `false` | Apply read-only sandboxing                               |
| `--ro-paths`    | `-O`  |         | Additional read-only paths for sandboxing                |
| `--rw-paths`    | `-W`  |         | Additional read-write paths for sandboxing               |
| `--headless`    | `-H`  | `false` | No printouts during sandbox setup                        |
| `--with-mcp`    |       | `false` | Connect to MCP servers and include their tools           |
| `--mcp-servers` |       |         | Only connect to named MCP servers (implies `--with-mcp`) |

Plus all [global flags]({{ site.baseurl }}/commands/#global-flags).

## Plugin Behavior

`aura tools` loads plugins and registers **plugin tools** (custom tools like `Gotify`, `Notepad` are fully available). However, **plugin hooks** (`BeforeToolExecution`, `AfterToolExecution`, etc.) do **not** fire — `aura tools` calls `Execute()` directly, bypassing the injector pipeline. Built-in features that live inside `Execute()` (like `bash.rewrite`) still apply.

## Examples

```sh
# List all tools
aura tools

# Inspect the Read tool
aura tools Read

# Execute with key=value pairs
aura tools Read path=main.go line_start=1 line_end=10

# Execute with JSON
aura tools Read '{"path": "main.go"}'

# Execute with sandboxing
aura tools --ro Read path=/etc/passwd

# Force JSON parsing
aura tools Read --raw '{"path": "main.go"}'

# JSON output for scripting
aura tools --json

# List all tools including MCP
aura tools --with-mcp

# Execute an MCP tool
aura tools --with-mcp mcp__context7__resolve-library-id query="go testing" libraryName="go"

# Only connect to specific MCP servers
aura tools --mcp-servers context7 --mcp-servers github
```
