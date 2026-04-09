---
layout: default
title: Configuration
nav_order: 4
has_children: true
---

# Configuration

All configuration lives under `.aura/config/` as YAML and Markdown files. Bootstrap the default configuration with:

```sh
aura init
```

## Directory Structure

```
.aura/
├── config/
│   ├── agents/           # Agent definitions (Markdown + YAML frontmatter)
│   │   └── features/     # Hidden agents used by internal features (e.g. embeddings, guardrail)
│   ├── commands/         # Custom slash commands (Markdown + YAML frontmatter)
│   ├── features/         # Feature configs (YAML)
│   ├── hooks/            # User hook definitions (YAML) — pre/post tool execution
│   ├── lsp/              # LSP server definitions (YAML)
│   ├── mcp/              # MCP server definitions (YAML)
│   ├── modes/            # Mode definitions (Markdown + YAML frontmatter)
│   ├── prompts/          # System prompts (Markdown + YAML frontmatter, supports `inherit:`)
│   │   ├── system/       # Main system prompts (referenced by agent `system:` field)
│   │   ├── features/     # Feature-specific prompts (compaction, thinking)
│   │   └── logs/         # Logging-specific prompts
│   ├── providers/        # Provider configs (YAML)
│   ├── rules/            # Persistent approval rules (created on-demand when approvals are saved)
│   ├── sandbox/          # Landlock sandbox config (YAML) — merged into features sandbox
│   ├── tasks/            # Scheduled task definitions (YAML)
│   └── tools/            # Optional tool text overrides (any tool type)
├── plugins/              # Go plugins (recursive: organize into subdirs freely)
├── sessions/             # Saved sessions ({uuid}.json)
├── skills/               # LLM-invocable skills (Markdown + YAML frontmatter)
├── embeddings/           # Embeddings index
├── memory/               # Persistent key-value memory files
└── debug.log             # Debug log (when --debug is enabled)
```

Files under each config type (e.g. `agents/`, `prompts/`) are loaded recursively — subfolder organization does not affect loading.

## Name Resolution

All name-based lookups (agents, modes, prompts, providers, commands, MCP servers, plugins, hooks, LSP servers, slash commands) are **case-insensitive**. For example, `/agent high` and `/agent High` resolve to the same agent. Tool and model names remain case-sensitive (LLM-facing contracts).

## Sessions

Sessions are stored as JSON files at `.aura/sessions/{uuid}.json`. Each session contains the conversation history, metadata (title, agent, mode, model, provider, thinking state, loaded tools, sandbox state, read-before policy, session approvals, stats, cumulative usage), and todo list. Manage sessions with `/save`, `/resume`, `/name`, and `/fork`.

## Embeddings Index

The embeddings index is stored at `.aura/embeddings/`. It is automatically created and incrementally updated when using the Query tool, `/query` command, or `aura query` CLI.

## Workspace Instructions

Aura automatically loads `AGENTS.md` files into the system prompt. When `--workdir` is used, it walks upward from the working directory, collecting `AGENTS.md` files at each level until it reaches a `.git` directory, the launch directory, or the filesystem root. Without `--workdir`, only the current directory and config homes are checked.

## Example Files

Each config directory includes a `.example.` reference file (e.g. `agent.example.md`, `provider.example.yaml`) that documents every possible field. These files are **not loaded** by Aura — they exist purely as documentation. To use one as a starting point, copy and rename it (removing `.example.` from the name).

## Runtime Overrides

The `--override` / `-O` flag sets config values from the command line using dot-notation, without editing YAML files. It's repeatable and has the highest priority in the config chain — overriding YAML files, agent frontmatter, mode settings, and other CLI flags like `--max-steps`.

```sh
# Override features
aura --override features.tools.max_steps=5 run "quick task"
aura -O features.compaction.threshold=0.9 -O features.guardrail.mode=block run "..."

# Override model settings
aura --override model.name=qwen3:8b run "explain this"
aura --override model.generation.temperature=0.1 run "be precise"
aura --override model.context=200000 run "large context task"
```

Two sections are supported:
- `features.*` — all 14 feature sub-structs (~80 fields): tools, compaction, guardrail, sandbox, thinking, vision, etc.
- `model.*` — name, provider, think, context, generation params (temperature, top_p, etc.)

Unknown fields produce a clear error at startup. Zero values work correctly (`features.tools.max_steps=0` sets max_steps to 0, not "unset").

## Environment Variables

All CLI flags support `AURA_` prefixed environment variables (e.g. `AURA_AGENT`, `AURA_MODEL`, `AURA_DEBUG`). Precedence: CLI flag > `AURA_*` env var > default. Use `aura --print-env` to see all resolved settings.

Provider tokens follow the pattern `AURA_PROVIDERS_{NAME}_TOKEN` (e.g. `AURA_PROVIDERS_OPENROUTER_TOKEN`). Tokens can also be set in provider YAML configs or loaded from an env file via `--env-file` (default: `secrets.env`).
