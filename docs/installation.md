---
layout: default
title: Installation
nav_order: 2
---

# Installation

## Prebuilt Binary

```sh
curl -sSL https://raw.githubusercontent.com/idelchi/aura/refs/heads/dev/install.sh | sh -s -- -d ~/.local/bin
```

## From Source

```sh
go install github.com/idelchi/aura@latest
```

## Provider Setup

Aura requires at least one LLM provider. Configure providers in `.aura/config/providers/`.

Supported providers:

| Provider                                            | Type  | Default URL                                  | URL Override |
| --------------------------------------------------- | ----- | -------------------------------------------- | ------------ |
| [Ollama](https://ollama.ai)                         | Local | None (must be set in YAML)                   | Yes          |
| [llama.cpp](https://github.com/ggerganov/llama.cpp) | Local | None (must be set in YAML)                   | Yes          |
| [OpenRouter](https://openrouter.ai)                 | Cloud | `https://openrouter.ai/api/v1`               | Yes          |
| [OpenAI](https://openai.com)                        | Cloud | `https://api.openai.com/v1`                  | Yes          |
| [Anthropic](https://anthropic.com)                  | Cloud | `https://api.anthropic.com`                  | Yes          |
| [Google Gemini](https://ai.google.dev)              | Cloud | `https://generativelanguage.googleapis.com/` | Yes          |
| GitHub Copilot                                      | Cloud | Dynamic (token exchange with GitHub)         | No           |
| OpenAI Plus (Codex)                                 | Cloud | `https://chatgpt.com/backend-api/codex`      | Yes          |

Copilot does not accept URL overrides — its URL is resolved dynamically via GitHub's token exchange API.

See [Provider Configuration]({{ site.baseurl }}/configuration/providers/) for YAML setup and token configuration.

## Initialize Configuration

Scaffold the default configuration directory:

```sh
aura init
```

This creates `.aura/config/` with all default agents, modes, providers, features, and prompts. See [Configuration]({{ site.baseurl }}/configuration/) for details.

## Environment Variables

All CLI flags can be set via `AURA_` prefixed environment variables. Precedence: CLI flag > `AURA_*` env var > default value.

```sh
# Examples
export AURA_AGENT=high
export AURA_MODEL=llama3:8b
export AURA_DEBUG=1

# See all resolved settings
aura --print-env
```

Tokens are resolved as `AURA_PROVIDERS_<PROVIDER>_TOKEN`. As an example, defining a provider `ollama` would look for `AURA_PROVIDERS_OLLAMA_TOKEN`.
Tokens can also be set in provider YAML configs or loaded from an env file via `--env-file` (default: `secrets.env`).
