---
layout: default
title: Tool Definitions
parent: Configuration
nav_order: 5
---

# Tool Definitions

Tool behavior can be customized via YAML files in `.aura/config/tools/`. The schema covers text overrides and disabling for any tool type — built-in, plugin, Task, Ask, and Done.

## How It Works

The loader collects **all** `**/*.yaml` files under `.aura/config/tools/` (recursively), parses each as a map of tool names to definitions, and merges them. File names are irrelevant — a single file can define overrides for multiple tools. The convention is one file per tool, but it's not enforced.

The tool name is always the **map key**, never a `name:` field inside the definition.

## YAML Schema

```yaml
tool_name:
  disabled: false # Remove this tool entirely
  override: false # Replace a built-in tool with a plugin of the same name
  condition: "" # Condition expression — tool excluded when false
  parallel: true # Override parallel execution safety (nil = use code default)
  description: |
    What the tool does. Shown to the LLM in the tool definition.
  usage: |
    How and when to use the tool. Guidance for the LLM.
  examples: |
    {"param": "value"}
    {"param": "value", "other": 123}
```

## Text Overrides

Non-empty fields override the compiled-in default; fields you omit keep the default. This works for all tool types — built-in, plugin, Task, Ask, and Done.

```yaml
# .aura/config/tools/bash.yaml
bash:
  description: |
    Run shell commands. Prefer dedicated tools (Read, Glob, Patch) over Bash
    for file operations. Never prefix commands with 'bash' or 'bash -lc'.
```

Multiple tools in one file:

```yaml
# .aura/config/tools/readonly.yaml
read:
  usage: |
    Always read the full file unless it exceeds 500 lines.
glob:
  usage: |
    Prefer exact paths over broad patterns.
```

## Disabling Tools

Set `disabled: true` to remove a tool entirely:

```yaml
mkdir:
  disabled: true
```

This removes the tool from all agents and modes. It will not appear in `aura tools` or be available to the LLM.

## Conditional Inclusion

Set `condition:` to a condition expression — the tool is excluded when the condition evaluates to false. Conditions are re-evaluated every turn.

```yaml
vision:
  condition: "model_has:vision"

query:
  condition: "model_params_gt:7"

local-only:
  condition: "model_is:granite4:micro-h"
```

Conditions use the same expression syntax as `/assert` — see [Slash Commands]({{ site.baseurl }}/features/slash-commands#assert) for the full expression reference. Invalid condition expressions are rejected at config load time.

If an agent or mode explicitly enables a tool by name (not bare `*`), the explicit enable overrides the condition.

ToolDef conditions also apply to MCP tools. If an MCP tool has a ToolDef entry with a `condition:`, it takes precedence over the server-level condition.

## Parallel Override

Set `parallel:` to override a tool's code-level parallel execution safety. When parallel execution is enabled (`tools.parallel` is omitted or `true`, the default), tools that declare themselves parallel-safe run concurrently. This field lets you override that per-tool without changing code. These overrides apply to both the main pipeline and [Batch]({{ site.baseurl }}/features/tools#batch) sub-calls.

```yaml
# Force Bash to run sequentially even though its code says parallel-safe:
bash:
  parallel: false

# Force a normally-sequential tool to run in parallel:
myslowreader:
  parallel: true
```

Resolution chain (first non-nil wins):

1. **Global toggle** — `tools.parallel: false` in `features/tools.yaml` disables all parallelism (no per-tool checks).
2. **ToolDef override** — `parallel: true/false` in tools YAML overrides the tool's code default.
3. **Code default** — the tool's `Parallel()` method (most tools return `true`).

Omitting `parallel:` (or not having a ToolDef entry) defers to the code default.

## Custom Tools

For custom tools, define them as Go plugins — see [Plugins]({{ site.baseurl }}/features/plugins).

## Defaults

No YAML override files ship by default — all tool descriptions, usage, and examples are compiled into the Go source. Override files are purely optional customizations. See `.aura/config/tools/tool.example.yaml` for the schema.
