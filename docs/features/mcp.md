---
layout: default
title: MCP
parent: Features
nav_order: 8
---

# MCP Support

Aura supports the Model Context Protocol (MCP) for connecting to external tool servers.

## Configuration

MCP servers are defined in `.aura/config/mcp/*.yaml`:

```yaml
server-name:
  type: http
  url: https://example.com/mcp
  headers:
    Authorization: Bearer ${API_TOKEN}

local-tool:
  type: stdio
  command: go
  args: [run, ./mcp-server/main.go]

disabled-server:
  type: http
  url: https://example.com/mcp
  disabled: true
```

## Transport Types

| Type    | Fields                              | Description                 |
| ------- | ----------------------------------- | --------------------------- |
| `http`  | `url`, `headers`, `timeout`         | HTTP-based MCP server       |
| `stdio` | `command`, `args`, `env`, `timeout` | Subprocess via stdin/stdout |

Header values support `${VAR_NAME}` env var expansion.

## Deferred Tool Loading

Set `deferred: true` to exclude a server's tools from the initial tool set. Tools load on demand via the `LoadTools` meta-tool, reducing context usage in MCP-heavy setups. The server still connects at startup — only tool registration is deferred.

Alternatively, defer individual MCP tools via glob patterns in `features/tools.yaml` instead of deferring the entire server: `deferred: ["mcp__github__*"]`.

```yaml
github:
  deferred: true
  command: npx
  args: ["-y", "@modelcontextprotocol/server-github"]
  env:
    GITHUB_TOKEN: ${GITHUB_TOKEN}
```

## Server Filtering

Filter which servers connect using glob patterns — same system as tool filtering.

```bash
aura run --include-mcps "context7,git*"
aura run --exclude-mcps "portainer"
aura run --exclude-mcps "*"   # disable all MCP
```

Feature config defaults (`features/mcp.yaml`):

```yaml
mcp:
  enabled: [] # empty = connect all
  disabled: ["portainer"]
```

CLI flags override feature config. Exclude takes precedence over include. Per-server `disabled: true` is an author-side toggle; filtering is a user-side override.

Filters are reapplied on agent switch (if MCP filter rules differ) and on config reload. Use `/reload` to fully reconnect all servers from scratch.

## Conditional Inclusion

Set `condition:` on a server to exclude its tools based on runtime state. The server still connects — only tool visibility is affected. Conditions are re-evaluated every turn.

```yaml
heavy-server:
  condition: "model_params_gt:7"
  type: http
  url: https://example.com/mcp
```

Conditions use the same expression syntax as `/assert` — see [Slash Commands]({{ site.baseurl }}/features/slash-commands#assert).

## MCP Tools

MCP tools appear with the `mcp__` prefix — e.g., `mcp__github__search`. Use `/mcp` to list connected servers and tools, `/mcp reconnect` to retry failed servers.

Agent and mode configs can filter MCP tools:

```yaml
tools:
  disabled:
    - mcp__* # Disable all MCP tools
```
