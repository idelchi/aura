---
layout: default
title: LSP
parent: Features
nav_order: 7
---

# LSP

Aura integrates with Language Server Protocol servers to provide real-time compiler diagnostics after tool execution. Errors and warnings are appended to tool results so the LLM can observe and fix them without an explicit step.

## Configuration

```
.aura/config/lsp/*.yaml
```

Each file is a YAML map of server names to definitions.

| Field          | Type     | Default | Description                                                    |
| -------------- | -------- | ------- | -------------------------------------------------------------- |
| `command`      | string   |         | Binary name or path to the language server                     |
| `args`         | []string | `[]`    | Command-line arguments                                         |
| `file_types`   | []string | `[]`    | File extensions handled (e.g. `[go, mod]`). Empty = all        |
| `root_markers` | []string | `[]`    | Files that must exist in working directory to start the server |
| `settings`     | object   | `{}`    | Workspace settings via `workspace/didChangeConfiguration`      |
| `init_options` | object   | `{}`    | LSP initialization options sent during `initialize`            |
| `timeout`      | int      | `30`    | Seconds to wait for the server to initialize                   |
| `disabled`     | bool     | `false` | Skip without removing config                                   |

`root_markers` prevents starting servers in unrelated projects — e.g., `root_markers: [go.mod]` only starts in Go projects.

## Tools

LSP provides two opt-in tools:

- **Diagnostics** — returns errors and warnings from all active LSP servers. Accepts optional `path` parameter (omit for all open files). Servers start lazily on first use.
- **LspRestart** — restarts LSP servers. Accepts optional `server` name (omit to restart all).

Both are hidden unless explicitly enabled:

```yaml
# In agent frontmatter or features/tools.yaml
tools:
  enabled: [Diagnostics, LspRestart]
```

See [Opt-In Tools]({{ site.baseurl }}/features/tools#opt-in-tools).

## Diagnostic Pipeline

Diagnostics run **after hooks** complete, so formatters and fixers have already modified files before the LSP server evaluates them:

1. Tool executes (e.g., `Patch`, `Write`)
2. Post hooks run (formatters, fixers)
3. LSP diagnostics collected for modified files
4. All feedback appended to the tool result

## Example Configuration

```yaml
# .aura/config/lsp/gopls.yaml
gopls:
  command: gopls
  args: [serve]
  file_types: [go, mod]
  root_markers: [go.mod]
  settings:
    directoryFilters: ["-**/vendor", "-**/node_modules", "-.git"]
    staticcheck: true
```

Servers start lazily, run for the session duration, and stop on exit. `LspRestart` can recover a misbehaving server without restarting Aura.
