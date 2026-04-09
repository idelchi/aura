---
layout: default
title: Packages
parent: Contributing
nav_order: 4
---

# Packages

## Layout

Aura's source splits into three top-level areas:

| Area        | Importable externally   | Purpose                                                                      |
| ----------- | ----------------------- | ---------------------------------------------------------------------------- |
| `internal/` | No                      | Application logic — CLI, assistant loop, config, tools, plugins, UI          |
| `pkg/`      | Yes                     | Reusable libraries — LLM types, provider implementations, sandboxing, tokens |
| `sdk/`      | Yes (separate `go.mod`) | Plugin contract — types, symbols, and version negotiation for Yaegi plugins  |

## internal/

| Category   | Packages                                                                                     | Purpose                                                                                             |
| ---------- | -------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| CLI        | `cli/`, `cli/core/`, `cli/packs/` + 14 subcommand packages                                   | urfave/cli v3 commands, flag wiring, shared git-workflow ops                                        |
| Core Loop  | `assistant/`, `conversation/`, `session/`                                                    | Assistant loop, message history/builder, session persistence                                        |
| Config     | `config/`, `config/inherit/`, `config/merge/`                                                | YAML + frontmatter loading, DAG inheritance, canonical struct merge                                 |
| Tools      | `tools/` (23 tool packages)                                                                  | Built-in tool implementations                                                                       |
| Extensions | `plugins/`, `injector/`, `hooks/`, `mcp/`, `lsp/`, `slash/`                                  | Plugin runtime, injection registry, shell hooks, MCP servers, LSP client management, slash commands |
| Features   | `agent/`, `prompts/`, `directive/`, `condition/`, `task/`, `subagent/`, `todo/`, `snapshot/` | Agent resolution, prompt rendering, input directives, conditions, tasks                             |
| Search     | `embedder/`, `indexer/`, `chunker/`                                                          | Embeddings pipeline (embed → index → chunk)                                                         |
| UI         | `ui/` (tui, simple, headless, web)                                                           | Four UI backends (Bubble Tea, readline, headless, htmx)                                             |
| Support    | `debug/`, `mirror/`, `diffpreview/`, `spintext/`, `stats/`, `tmpl/`                          | Debug logging, output mirroring, diff display, spinner text, statistics, template expansion         |

## pkg/

| Category   | Packages                                                                                                                                                                                                          | Purpose                                                                 |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| LLM Domain | `llm/` + 15 subpackages (`message/`, `model/`, `tool/`, `usage/`, `part/`, `request/`, `stream/`, `roles/`, `thinking/`, `generation/`, `responseformat/`, `embedding/`, `rerank/`, `transcribe/`, `synthesize/`) | Shared types for messages, models, tools, usage, streaming              |
| Providers  | `providers/` + 8 implementations (`ollama/`, `llamacpp/`, `openrouter/`, `openai/`, `anthropic/`, `google/`, `copilot/`, `codex/`) + `adapter/`, `retry/`                                                         | LLM backends, Fantasy adapter, retry decorator                          |
| Utilities  | `auth/`, `clipboard/`, `frontmatter/`, `gitutil/`, `glob/`, `image/`, `sandbox/`, `spinner/`, `tokens/`, `truncate/`, `truthy/`, `wildcard/`, `yamlutil/`                                                         | OAuth, clipboard, YAML parsing, git detection, Landlock, token counting |

## sdk/

Separate Go module (`sdk/go.mod`) with zero external dependencies. Defines:

- **Types** (`sdk/sdk.go`) — `Result`, `Context`, `ToolCall`, hook context structs (`BeforeChatContext`, `AfterToolContext`, etc.), modification types
- **Symbols** (`sdk/symbols.go`) — Yaegi reflection map for runtime type resolution
- **Version** (`sdk/version/version.go`) — SDK version for compatibility checking between plugins and host

Plugins import the SDK module directly. The host registers symbols via `interp.Use(sdk.Symbols)`.

## Where to Put New Code

1. **New built-in tool?** → `internal/tools/<name>/`
2. **New LLM provider?** → `pkg/providers/<name>/`
3. **New config entity?** → `internal/config/<name>.go`
4. **New CLI subcommand?** → `internal/cli/<name>/`
5. **Reusable library with no `internal/` imports?** → `pkg/<name>/`
6. **Plugin-visible type or hook context?** → `sdk/sdk.go` + `sdk/symbols.go`
7. **Everything else** → add to the nearest existing package in `internal/`

Create a new package when the code has a distinct, self-contained concern and would have 3+ files or 200+ lines. Otherwise, add to an existing package.

## Naming Conventions

- **Package names**: lowercase, single word when possible (`config`, `tools`, `slash`)
- **Method naming**: no `Get`/`Set` prefixes. `Name()` not `GetName()`. Booleans: `IsRunning()`, `HasTools()`. Prefer natural verbs for setters: `MarkRunning()`, `Fail(err)`, `UseReranker(p)`
- **Display methods**: `String()` for Go stringer (`%s`), `Display()` for rich human-facing output (colors, padding), `Render()` reserved for conversation rendering (messages → text)
- **Information Expert**: put behavior on the type that owns the data. Formatting/display methods live on the domain struct, not in consuming packages
