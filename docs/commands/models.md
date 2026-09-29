---
layout: default
title: aura models
parent: Commands
nav_order: 2
---

# aura models

List available models from the configured provider.

## Syntax

```sh
aura models [flags]
```

## Description

Lists all available models from the configured provider with optional sorting. Model visibility can be controlled per-provider using `models.include` and `models.exclude` glob patterns in the provider config.

Model lists are cached per provider connection in `.aura/cache/models/`. Metadata learned when Aura inspects a selected model is saved in `.aura/cache/model-metadata/` and reused by listings, including `/model`, after the model is unloaded. Fresh explicit capability reports take precedence over saved values. Listing models does not load or swap workers to discover capabilities.

`T` means thinking is supported and `V` means vision is supported. `T?` and `V?` mean the provider has not reported that capability; a blank means it is known to be unsupported. Models with unreported tool support appear under **Tool capability unknown**, rather than **Other**. Capability filters include only confirmed support.

Use `--no-cache` to bypass both catalog and metadata caches and re-fetch from providers (fresh data is still written back to cache). Unknown capabilities can therefore reappear with `--no-cache`. Use `aura cache clean` to delete all cached data.

## Flags

| Flag        | Short | Default | Description                                                                                         |
| ----------- | ----- | ------- | --------------------------------------------------------------------------------------------------- |
| `--sort-by` |       | `size`  | Sort by: `name` (alphabetical), `context` (context window length), `size` (model file size on disk) |
| `--filter`  | `-f`  |         | Filter by capability (combinable): `thinking`, `tools`, `embedding`, `reranking`, `vision`          |
| `--name`    | `-n`  |         | Filter by model name (wildcard patterns, repeatable)                                                |

Plus all [global flags]({{ site.baseurl }}/commands/#global-flags).

## Examples

```sh
# List models from default provider (Ollama)
aura models

# Sort by context window size
aura models --sort-by context

# List models from specific providers
aura --providers openrouter models

# List models from multiple providers
aura --providers ollama,openrouter models

# Filter by capability
aura models --filter vision
aura models --filter thinking,vision

# Filter by model name (wildcard)
aura models --name='gemma*'
aura models --name='qwen*' --name='llama*'

# Combine name and capability filters
aura models --name='gemma*' --filter vision

# Bypass cache and re-fetch from providers
aura --no-cache models
```

> **Note**: The global `--providers` flag filters model listings and silently discards agents whose provider is not in the list. The chosen agent's provider must be included — otherwise it's a hard error. Model visibility filters in provider configs control which models appear within each provider.
