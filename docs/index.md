---
layout: default
title: Home
nav_order: 1
description: "Aura — Agentic coding CLI"
permalink: /
---

# Aura

{: .fs-9 }

Agentic coding CLI.
{: .fs-6 .fw-300 }

[Get Started]({{ site.baseurl }}/quickstart){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 } [View on GitLab](https://github.com/idelchi/aura){: .btn .fs-5 .mb-4 .mb-md-0 }
![Aura in Action]({{ site.baseurl }}/assets/gifs/run.gif)
![Aura in Action]({{ site.baseurl }}/assets/gifs/aura.gif)

---

`aura` is a terminal-based coding assistant that connects to local or remote LLMs. Agents, tools, modes, guardrails, and providers are defined as YAML and Markdown files.

```sh
# Start interactive session
aura

# One-off prompt
aura run "Write a Go function that reverses a string"

# Embedding-based codebase search
aura query "token counting"

# List available models
aura models
```

## Task Orchestration

Tasks are YAML files that sequence prompts, slash commands, and shell commands. `/assert` and `/until` add condition gates — the LLM keeps working until the condition is satisfied.

```yaml
build-app:
  agent: high
  timeout: 60m
  commands:
    - /mode plan
    - Read SPEC.md and generate a plan.
    - /until not todo_empty "Create the plan with TodoCreate"
    - /mode edit
    - /auto on
    - Execute the plan.
    - /until bash:"go build ./..." "Build is failing. Fix the errors."
```

Tasks support cron scheduling, foreach iteration over files or shell output, pre/post shell hooks, session continuity across runs, and template variables. See [Examples]({{ site.baseurl }}/examples) for more patterns.

## Providers

Aura connects to local and remote LLM providers. No single vendor required — switch providers per agent, per task, or at runtime.

- Ollama
- LlamaCPP
- OpenRouter
- OpenAI
- Anthropic
- Google
- Copilot
- Codex

## Configuration

Everything is file-based YAML and Markdown. Agents, modes, prompts, hooks, and tasks support `inherit:` with DAG-based resolution

## Extensibility

**Go plugins** — interpreted Go code (Yaegi) with lifecycle hooks at 8 timings, custom tools with sandbox integration, and custom slash commands. Distributed via git with vendored dependencies.

## Features

| Feature         | Description                                                |
| --------------- | ---------------------------------------------------------- |
| Agents          | Per-agent model, provider, system prompt, and tool filters |
| Tools           | Built-in tools + custom tools via Go plugins               |
| Modes           | Tool availability and bash command restrictions            |
| Guardrails      | Secondary LLM validation of tool calls and user messages   |
| Slash Commands  | Built-in + user-defined as Markdown files                  |
| Skills          | LLM-invocable capabilities with progressive disclosure     |
| Compaction      | Automatic context compression via dedicated agent          |
| Embeddings      | Embedding-based codebase search with AST-aware chunking    |
| Sessions        | Save, resume, and fork conversations                       |
| Sandboxing      | Landlock LSM filesystem restrictions                       |
| MCP             | HTTP and STDIO transports                                  |
| Thinking        | Extended reasoning with configurable levels                |
| Vision          | Image/PDF analysis via vision-capable model delegation     |
| Audio           | Speech-to-text transcription and text-to-speech synthesis  |
| Hooks           | Shell commands before/after tool execution                 |
| LSP             | Language server diagnostics appended to tool results       |
| Plugins         | User-defined Go plugins via Yaegi interpreter              |
| Memory          | Persistent key-value storage across sessions               |
| Deferred Tools  | On-demand tool loading to reduce initial context usage     |
| Scheduled Tasks | Cron-based task scheduling with foreach iteration          |
| Web UI          | Browser-based chat interface with SSE streaming            |
| Auto Mode       | Continuous execution with condition gates and Done tool    |
