---
layout: default
title: aura mcp
parent: Commands
nav_order: 10
---

# aura mcp

List configured MCP servers and their discovered tools.

```sh
aura mcp
```

## Behavior

1. Loads all MCP server definitions from `.aura/config/mcp/*.yaml`
2. Applies MCP filtering (feature config `mcp.enabled`/`mcp.disabled` and CLI `--include-mcps`/`--exclude-mcps`)
3. Connects to all enabled servers in parallel
4. Displays each server's name, type, endpoint, enabled/disabled status, and available tools

Disabled servers show their configuration but are not connected. Connection errors are displayed inline.

## Output Format

```
SERVER-NAME (type)
  http://example.com/mcp
  tools:
    - tool_one
    - tool_two

OTHER-SERVER (stdio) [disabled]
  my-mcp-binary --port 3000
```

## Examples

```sh
# List all configured servers and their tools
aura mcp

# With MCP filtering (global flags must appear before subcommand)
aura --include-mcps "context7,github" mcp
aura --exclude-mcps "portainer" mcp
```

## In-Chat Command

The `/mcp` slash command provides the same listing inside an interactive session, plus the ability to reconnect failed servers:

```
/mcp                      # List servers and tools
/mcp reconnect            # Reconnect all failed servers
/mcp reconnect <server>   # Reconnect a specific server
```

See [/mcp — MCP Server Management]({{ site.baseurl }}/features/slash-commands#mcp--mcp-server-management) for details.

## See Also

- [MCP feature configuration]({{ site.baseurl }}/features/mcp)
- [MCP server definitions]({{ site.baseurl }}/configuration#mcp-servers)
