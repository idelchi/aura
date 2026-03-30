---
layout: default
title: Utility Commands
parent: Commands
nav_order: 16
---

# Utility Commands

## aura init

Scaffold a default Aura configuration into the current directory.

```sh
aura init [flags]
```

| Flag    | Short | Default | Description                        |
| ------- | ----- | ------- | ---------------------------------- |
| `--dir` | `-d`  | `.aura` | Output directory for configuration |

```sh
# Initialize default config
aura init

# Initialize to a custom directory
aura init -d my-config
```

Existing files are not overwritten. See [Configuration]({{ site.baseurl }}/configuration/) for details on each config section.

---

## aura web

Start a browser-based chat UI over HTTP with SSE streaming.

```sh
aura web [flags]
```

| Flag     | Default          | Description     |
| -------- | ---------------- | --------------- |
| `--bind` | `127.0.0.1:9999` | Address to bind |

```sh
# Start on default port
aura web

# Bind to a specific port
aura web --bind 127.0.0.1:8080
```

Binds to `127.0.0.1` by default — place behind a reverse proxy for remote access. Session state survives reloads; `--continue` and `--resume` global flags work here too.

---

## aura login

Authenticate with an OAuth provider via device code flow.

```sh
aura login <provider> [flags]
```

| Flag      | Default | Description                                             |
| --------- | ------- | ------------------------------------------------------- |
| `--local` | `false` | Save token to project config instead of `~/.aura/auth/` |

Supported providers: `copilot` (GitHub OAuth `ghu_...`) and `codex` (OpenAI refresh token `rt_...`).

```sh
aura login copilot
```

Tokens can also be set via environment variables (`AURA_PROVIDERS_COPILOT_TOKEN`, `AURA_PROVIDERS_CODEX_TOKEN`) or in the provider YAML config.

---

## aura cache

Manage cached provider metadata and model lists.

```sh
aura cache clean
```

Aura caches data in `.aura/cache/` with no expiry. `clean` deletes the entire cache directory; all data is re-fetched on next use.

```sh
aura cache clean
```

Use the `--no-cache` global flag to bypass cache reads for a single invocation without deleting the cache.

---

## aura query

Embedding-based search across the codebase.

```sh
aura query [terms...]
```

| Flag    | Short | Default     | Description                     |
| ------- | ----- | ----------- | ------------------------------- |
| `--top` | `-k`  | from config | Number of top results to return |

```sh
aura query "token counting"
```

Running without arguments reindexes without searching. The index lives in `.aura/embeddings/`. See [Embeddings]({{ site.baseurl }}/features/embeddings) for configuration.

---

## aura vision

Analyze an image or PDF via a vision-capable LLM.

```sh
aura vision <file> [instruction]
```

Requires a vision agent configured in `features/vision.yaml`. If no instruction is given, the model describes what it sees or extracts text.

```sh
aura vision diagram.jpg "Describe the architecture shown here"
```

---

## aura transcribe

Transcribe an audio file to text using a whisper-compatible STT server.

```sh
aura transcribe <file> [flags]
```

| Flag         | Short | Default     | Description                                     |
| ------------ | ----- | ----------- | ----------------------------------------------- |
| `--language` | `-l`  | auto-detect | ISO-639-1 language hint (e.g. `en`, `de`, `ja`) |

Requires an STT agent configured in `features/stt.yaml`. Supported formats: mp3, wav, ogg, flac, m4a, webm.

```sh
aura transcribe meeting.wav --language en
```

---

## aura speak

Convert text to speech audio using an OpenAI-compatible TTS server.

```sh
aura speak <text> <output> [flags]
```

| Flag      | Short | Default     | Description                                 |
| --------- | ----- | ----------- | ------------------------------------------- |
| `--voice` | `-V`  | from config | Voice identifier (e.g. `alloy`, `af_heart`) |

Requires a TTS agent configured in `features/tts.yaml`.

```sh
aura speak "Good morning" greeting.mp3 --voice af_heart
```
