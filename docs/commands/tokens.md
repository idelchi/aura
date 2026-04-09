---
layout: default
title: aura tokens
parent: Commands
nav_order: 9
---

# aura tokens

Count tokens in a file or stdin.

## Syntax

```sh
aura tokens [flags] <file>
cat file | aura tokens [flags]
```

## Description

Counts tokens using the configured estimation method from `features/estimation.yaml`. Reads input from a file path argument or stdin.

Multiple estimation methods can be specified for comparison. When a single method is used, output is a plain number. When multiple methods are specified, output is labeled (`method: count`).

Native estimation uses the current agent's provider. Use `--agent`, `--provider`, or `--model` root flags to control which provider is used, or omit to use the default agent.

Delegates to the built-in `Tokens` tool — the same tool the LLM uses via `aura run`.

## Flags

| Flag       | Default     | Description                                                                     |
| ---------- | ----------- | ------------------------------------------------------------------------------- |
| `--method` | from config | Estimation method (repeatable): `rough`, `tiktoken`, `rough+tiktoken`, `native` |

Plus all [global flags]({{ site.baseurl }}/commands/#global-flags).

## Methods

| Method           | Description                                                        |
| ---------------- | ------------------------------------------------------------------ |
| `rough`          | Character count divided by divisor (default 4)                     |
| `tiktoken`       | Token count via tiktoken encoding (default `cl100k_base`)          |
| `rough+tiktoken` | Maximum of rough and tiktoken estimates                            |
| `native`         | Provider's native tokenization using the resolved agent's provider |

## Examples

```sh
# Count tokens in a file (uses configured default method)
aura tokens path/to/file.go

# Count tokens from stdin
cat large-prompt.txt | aura tokens

# Override estimation method
aura tokens --method tiktoken path/to/file.go

# Compare multiple methods
aura tokens --method rough --method tiktoken path/to/file.go

# Native estimation (uses default agent's provider)
aura tokens --method native path/to/file.go

# Native with explicit provider/model
aura --provider ollama --model qwen3:32b tokens --method native path/to/file.go

# All methods at once
aura tokens --method rough --method tiktoken --method rough+tiktoken --method native path/to/file.go
```
