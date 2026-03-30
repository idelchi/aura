---
layout: default
title: Tools
parent: Features
nav_order: 1
---

{% raw %}

# Tools

Aura includes built-in tools that the LLM can invoke during conversations. Most are always present; `Skill` is registered only when skills are configured; `Done` and `Ask` are added dynamically per session.

## Built-in Tools

| Tool             | Description                                 | Sandboxable | Parallel |
| ---------------- | ------------------------------------------- | ----------- | -------- |
| **Bash**         | Shell execution (15s default timeout)       | Yes         | Yes      |
| **Ask**          | Prompt the user                             | No          | No       |
| **Read**         | Read file with optional line range          | Yes         | Yes      |
| **Write**        | Write file (full content)                   | Yes         | Yes      |
| **Edit**         | String replacement (old→new)                | Yes         | Yes      |
| **Glob**         | Recursive pattern match                     | Yes         | Yes      |
| **Ls**           | Directory listing                           | Yes         | Yes      |
| **Mkdir**        | Create directories                          | Yes         | Yes      |
| **Patch**        | Context-aware diff patching                 | Yes         | Yes      |
| **Rg**           | Regex search with line numbers              | Yes         | Yes      |
| **WebFetch**     | Fetch URL as markdown/text/HTML             | No          | Yes      |
| **WebSearch**    | Web search via DuckDuckGo                   | No          | Yes      |
| **TodoCreate**   | Create or update todo list                  | No          | No       |
| **TodoList**     | Show todo list                              | No          | Yes      |
| **TodoProgress** | Update todo item status                     | No          | No       |
| **Vision**       | Image/PDF analysis via vision LLM           | No          | Yes      |
| **Transcribe**   | Speech-to-text via whisper server           | No          | Yes      |
| **Speak**        | Text-to-speech via TTS server               | No          | Yes      |
| **Batch**        | Run multiple tool calls concurrently        | No          | Yes      |
| **Task**         | Delegate to a subagent (opt-in)             | No          | Yes      |
| **LoadTools**    | Load deferred tool schemas on demand        | No          | No       |
| **MemoryRead**   | Read/list/search persistent memory          | No          | Yes      |
| **MemoryWrite**  | Persist notes to disk                       | No          | Yes      |
| **Tokens**       | Count tokens in a file or string            | Yes         | Yes      |
| **Diagnostics**  | LSP diagnostics for file/workspace (opt-in) | No          | Yes      |
| **LspRestart**   | Restart LSP servers (opt-in)                | No          | No       |
| **Query**        | Embedding-based codebase search             | No          | Yes      |
| **Done**         | Explicit task completion signal             | No          | No       |
| **Skill**        | Invoke LLM-callable skills by name          | No          | Yes      |

## Skills

Skills are LLM-invocable capabilities defined as Markdown files in `.aura/skills/`. Unlike slash commands (user-typed), skills are invoked by the LLM via the `Skill` tool.

Only skill names and one-line descriptions are visible in the tool schema. The full body is returned only when invoked — token overhead stays flat regardless of how many skills exist.

```yaml
---
name: commit
description: Review staged changes and create a git commit with a meaningful message
---
Review all staged and unstaged changes using git status and git diff.
Draft a concise commit message that summarizes the changes.
Create the commit.
```

The `Skill` tool registers when at least one skill file exists and deregisters on `/reload` if all are removed. Skills load from `.aura/skills/**/*.md`.

## Memory

`MemoryRead` and `MemoryWrite` provide persistent key-value storage backed by markdown files. Memory survives across sessions and compactions.

| Scope             | Path              | Purpose                |
| ----------------- | ----------------- | ---------------------- |
| `local` (default) | `.aura/memory/`   | Project-specific notes |
| `global`          | `~/.aura/memory/` | Cross-project notes    |

`MemoryWrite` takes `key`, `content`, and optional `scope`. `MemoryRead` supports read by `key`, list all, or search by `query`.

## Patch Format

`*** Begin Patch` / `*** End Patch` markers with three operations: `*** Add File`, `*** Update File`, `*** Delete File`. Updates use `@@` context markers, `-` for removals, `+` for additions. Fuzzy context matching — exact line numbers not required.

    *** Begin Patch
    *** Update File: path/to/file.go
    @@ func main
    -old line
    +new line
    *** Delete File: obsolete.go
    *** End Patch

## Global Tool Filters

```yaml
# features/tools.yaml
tools:
  enabled: [] # glob patterns to include (empty = all)
  disabled: ["mcp__*"] # glob patterns to exclude
```

CLI flags override config when present:

```sh
aura --include-tools "Read,Glob,Rg,Ls"
aura --exclude-tools "Bash,Patch,Mkdir"
```

Patterns support wildcards (`*`, `Todo*`, `mcp__*`). Environment variables `AURA_INCLUDE_TOOLS` and `AURA_EXCLUDE_TOOLS` also work.

## Tool Filtering Pipeline

Each layer further restricts the set from the previous:

1. **Global** — `features/tools.yaml` or `--include-tools`/`--exclude-tools`. Modifies the base set for all subsequent layers.
2. **Agent** — `tools.enabled`/`tools.disabled` in agent frontmatter.
3. **Mode** — same fields in mode frontmatter.
4. **Task** — same fields in task definitions.
5. **Opt-in exclusion** — opt-in tools are dropped unless explicitly named at any prior layer.
6. **Deferred split** — tools matching `deferred` patterns are removed from the active set and listed in the system prompt instead.

### `opt_in` vs `disabled`

- **`disabled`**: Tool is removed. No layer can re-enable it.
- **`opt_in`**: Tool is hidden by default. Any layer can surface it by explicit name — not via `"*"` wildcard.

Run `/tools debug` to see all tools with their include/exclude status and the reason for each.

## Task

The `Task` tool delegates work to a subagent in an isolated context. Registered only when at least one agent has `subagent: true` in its frontmatter.

| Param         | Required | Description                                                     |
| ------------- | -------- | --------------------------------------------------------------- |
| `description` | yes      | Short summary (3–5 words)                                       |
| `prompt`      | yes      | Full task description                                           |
| `agent`       | no       | Subagent type; defaults to `default_agent`, then parent's agent |

Multiple `Task` calls in a single response execute in parallel.

## Batch

Executes multiple independent tool calls concurrently.

| Param   | Required | Description                                               |
| ------- | -------- | --------------------------------------------------------- |
| `calls` | yes      | Array of 1–25 sub-calls, each with `name` and `arguments` |

Partial failures do not stop other calls. Tool policy, guardrails, plugin hooks, sandbox checks, and user hooks all run per sub-call. Disallowed sub-tools: `Batch`, `Ask`, `Done`, `Task`, `LoadTools`.

## Opt-In Tools

Named in `opt_in` and hidden by default — the `"*"` wildcard does not surface them:

```yaml
# features/tools.yaml
tools:
  opt_in:
    - Ask
    - Done
    - Gotify
    - Diagnostics
    - LspRestart
    - Speak
    - Task
    - Transcribe
    - WebFetch
    - WebSearch
    - Write
```

Enable by explicit name at any layer:

```yaml
# tasks/notify.yaml
notify:
  tools:
    enabled:
      - Gotify
```

Plugin tools are also opt-in via `opt_in: true` in `plugin.yaml`.

## Deferred Tools

Tools matching `deferred` glob patterns in `features/tools.yaml`, or from an MCP server with `deferred: true`, are excluded from the active set. Their names are listed in the system prompt so the model knows they exist. Patterns work for any tool regardless of source — built-in, plugin, or MCP:

```yaml
deferred: ["Vision", "mcp__github__*", "mcp__portainer__*"]
```

`LoadTools` is automatically added when any deferred tools exist:

| Param   | Required | Description                                                      |
| ------- | -------- | ---------------------------------------------------------------- |
| `tools` | yes      | Tool names or glob patterns (e.g. `Vision`, `mcp__portainer__*`) |

Loaded schemas persist for the rest of the session.

## Tool Guards

- **Percentage mode** (default): Rejects tool results if projected context usage exceeds `result.max_percentage` (default: 95%).
- **Token mode**: Rejects results exceeding `result.max_tokens` (default: 20000).

User input messages are guarded separately: rejected if they would push context above `user_input_max_percentage` (default: 80%).

Additional per-tool guards:

- **`read_small_file_tokens`** (default: 2000) — Read ignores line range parameters and returns the full file when estimated token count is below this threshold.
- **`webfetch_max_body_size`** (default: 5 MiB) — maximum response body for WebFetch; larger responses are truncated.

Configure in `.aura/config/features/tools.yaml`. See [Features Config]({{ site.baseurl }}/configuration/features#tools).

## Read-Before Policy

Write and Patch enforce that existing files must be read before overwriting.

```yaml
tools:
  read_before:
    write: true # default: true
    delete: false # default: false
```

Toggle at runtime with `/readbefore` (alias `/rb`):

```
/readbefore                → show current state
/readbefore write off      → disable write enforcement
/readbefore delete on      → enable delete enforcement
/readbefore all off        → disable both
```

## Tool Definitions

YAML overrides in `.aura/config/tools/**/*.yaml` tune LLM prompts without recompiling; `disabled: true` removes a tool entirely. For custom tools, see [Extending Aura]({{ site.baseurl }}/contributing/extending).

## Execution Pipeline

1. **Pre-flight** — policy checks, guardrail validation, plugin hooks (can modify args or block execution)
2. **Execution** — parallel by default; Bash output streams to the UI at 200ms intervals
3. **Post-processing** — result size guards, hook file detection, diagnostics, plugin hooks

## Output Truncation

Bash output is truncated in two stages:

1. **Byte cap** — stdout and stderr each capped at `max_output_bytes` (default: 1MB).
2. **Line truncation** — output exceeding `max_lines` (default: 200) is middle-truncated; first `head_lines` and last `tail_lines` are kept with a separator showing the omitted count and a path to the full output.

```yaml
bash:
  truncation:
    max_output_bytes: 1048576
    max_lines: 200
    head_lines: 100
    tail_lines: 80
```

## Command Rewrite

`bash.rewrite` rewrites every Bash command before execution. The template receives `{{ .Command }}` and [sprig](https://masterminds.github.io/sprig/) functions.

```yaml
bash:
  rewrite: "rtk {{ .Command }}"
```

Common uses:

- Tool wrapping: `rtk {{ .Command }}`
- Environment setup: `source .venv/bin/activate && {{ .Command }}`
- Containerized execution: `docker exec -i mycontainer sh -c '{{ .Command }}'`
- Logging: `{{ .Command }} | tee /tmp/aura-bash.log`

## Failed Tool Call Pruning

When a tool call fails (tool not found, policy block, parse error), the error is injected as a tool result and pruned from history after one turn — preventing stale errors from permanently consuming context tokens.

## Global Tool Policy

A global default tool policy can be set in `features/tools.yaml`:

```yaml
tools:
  policy:
    auto: []
    confirm: []
    deny:
      - "Bash:sudo *"
      - "Bash:rm -rf *"
```

The effective policy is built additively: global `features.tools.policy` + agent `tools.policy` + mode `tools.policy` + persisted approval rules. Agent and mode policies are set in their respective `tools:` frontmatter blocks (see [Agents]({{ site.baseurl }}/configuration/agents) and [Modes]({{ site.baseurl }}/configuration/modes)).

Precedence within the merged policy: `deny > confirm > auto > default (auto)`.

## Persistent Approval Rules

When a tool requires confirmation and the user approves, the approval is saved at one of three scopes:

- **Session** — in-memory, cleared on exit
- **Project** — `.aura/config/rules/approvals.yaml`
- **Global** — `~/.aura/config/rules/approvals.yaml`

Approvals are scoped to the tool's primary argument where possible: Bash matches by command prefix, file tools by directory, others by tool name. Saved approvals merge into the `auto` tier of the tool policy.

## Parallel Execution

When the LLM emits multiple tool calls in one response, independent tools run concurrently. Non-parallel tools (Ask, Done, TodoCreate, TodoProgress, LoadTools, LspRestart) run after all parallel tools complete.

Disable globally:

```yaml
# features/tools.yaml
parallel: false
```

Override per-tool in [Tool Definitions]({{ site.baseurl }}/configuration/tools#parallel-override):

```yaml
# .aura/config/tools/bash.yaml
bash:
  parallel: false
```

## Max Steps

After `max_steps` iterations (default: 50), tools are disabled and the LLM must respond with text only. Override with `--max-steps`, `--override features.tools.max_steps=N`, or `max_steps:` in task definitions.

## Token Budget

`token_budget` sets a cumulative token limit (input + output). Once reached, the assistant stops immediately. Default: `0` (disabled). Override with `--token-budget` (env: `AURA_TOKEN_BUDGET`) or per-task in task definitions.

{% endraw %}
