---
layout: default
title: Providers
parent: Configuration
nav_order: 3
---

# Providers

Providers are YAML files in `.aura/config/providers/`. Each file configures a connection to an LLM backend.

## Provider Types

| Type         | Protocol                       | Capabilities                                                   |
| ------------ | ------------------------------ | -------------------------------------------------------------- |
| `ollama`     | Native Ollama API              | Chat, embedding, thinking, vision                              |
| `llamacpp`   | OpenAI-compatible              | Chat, reranking, thinking, vision, STT (whisper), TTS (kokoro) |
| `openrouter` | OpenRouter API                 | Chat, embedding (cloud models, token auth)                     |
| `openai`     | OpenAI Responses API           | Chat, embedding, transcription, synthesis                      |
| `anthropic`  | Native Anthropic Messages API  | Chat, thinking, vision, tools                                  |
| `google`     | Native Gemini API              | Chat, thinking, vision, tools, embedding                       |
| `copilot`    | GitHub Copilot (dual-protocol) | Chat (GPT via Responses API, Claude via Messages API)          |
| `codex`      | OpenAI Plus (Responses API)    | Chat (ChatGPT Plus/Pro subscription)                           |

All providers implement a 4-method core interface (Chat, Models, Model, Estimate). Optional capabilities use opt-in interfaces discovered via `providers.As[T](provider)`.

## YAML Schema

```yaml
provider_name:
  # Required for most providers; optional for anthropic/google (have defaults).
  url: http://host.docker.internal:11434

  # Determines which API protocol to use (required).
  # Values: ollama, llamacpp, openrouter, openai, anthropic, google, copilot, codex.
  type: ollama

  # Auth token. Falls back to AURA_PROVIDERS_{NAME}_TOKEN env var.
  # token: ""

  # How long models stay loaded in VRAM (Ollama only). Go duration syntax.
  # keep_alive: 15m

  # Wait for server to start responding. Does not affect streaming. Default: 5m.
  # timeout: 5m

  # Model visibility filter for `aura models` and `/model`.
  # Does NOT affect --model flag or feature agents.
  models:
    include: [] # Glob patterns — empty means all
    exclude: [] # Applied after include

  # Declared capabilities. Empty = all assumed.
  # Values: chat, embed, rerank, transcribe, synthesize.
  # capabilities: []

  # Retry for transient Chat() failures. Disabled by default (max_attempts: 0).
  # Only applies to ollama/llamacpp — other providers have built-in retry.
  retry:
    max_attempts: 0
    base_delay: 1s
    max_delay: 30s
```

## Token Resolution

1. `token` field in provider YAML — supports `$VAR` / `${VAR}` expansion
2. `AURA_PROVIDERS_{NAME}_TOKEN` environment variable
3. Value from `--env-file`

```yaml
openrouter:
  type: openrouter
  url: https://openrouter.ai/api/v1
  token: ${OPENROUTER_API_KEY}

anthropic:
  type: anthropic
  token: ${ANTHROPIC_API_KEY}
```

## Examples

```yaml
# Ollama (local)
my_ollama:
  url: http://host.docker.internal:11434
  type: ollama
  keep_alive: 15m
  models:
    exclude: ["*embed*"]

# OpenAI (also works for Groq, DeepSeek, or any /v1/responses-compatible service)
openai:
  url: https://api.openai.com/v1
  type: openai
  timeout: 5m

# Anthropic — URL defaults to https://api.anthropic.com
anthropic:
  type: anthropic
  timeout: 5m

# Google Gemini — URL defaults to Google's endpoint
google:
  type: google
  timeout: 5m

# GitHub Copilot — authenticate via `aura login copilot` or AURA_PROVIDERS_COPILOT_TOKEN
copilot:
  type: copilot
  timeout: 5m

# ChatGPT Plus/Pro — authenticate via `aura login codex` or AURA_PROVIDERS_CODEX_TOKEN
codex:
  type: codex
  timeout: 5m
```

---

## Catwalk Registry

Model capabilities (context length, vision, thinking levels) are enriched at startup using [Catwalk](https://catwalk.charm.sh) metadata. Aura ships with compiled-in embedded data for offline use. On startup it fetches fresh data and caches it to `.aura/cache/catwalk/`; on failure it falls back to the disk cache then the embedded data.

Enrichment only fills gaps — it never overwrites capabilities reported by the provider API. Applies to `anthropic`, `openai`, and `google` (their listing APIs return only model IDs). Other providers build capabilities inline from their own API responses.

Use `--no-cache` to force a fresh fetch, or `aura cache clean` to delete cached data.

---

Add as many providers as needed — create new YAML files in `providers/`. Any provider type can use a custom URL (remote Ollama instance, self-hosted OpenAI-compatible server, etc.).
