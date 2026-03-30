---
layout: default
title: Embeddings
parent: Features
nav_order: 4
---

# Embeddings

Embedding-based codebase search with optional reranking. Available through three interfaces:

- **Query tool** — LLM invokes it during conversations
- **`/query` command** — run from the slash command prompt
- **`aura query` CLI** — standalone command-line search

![Embeddings]({{ site.baseurl }}/assets/gifs/query.gif)

## Pipeline

```
Files → Indexer → Chunker → Embedder → Vector DB → Query → Reranker → Results
```

1. **Indexer** — Concurrent file walking (fastwalk, 20 workers) with SHA256 hash-based change detection. Only re-embeds files that have changed.
2. **Chunker** — Splits files into token-aware chunks with configurable overlap.
3. **Embedder** — Generates vector embeddings via the configured LLM provider.
4. **Vector DB** — chromem-go persistent vector database stored at `.aura/embeddings/`. Collections are keyed by embedding model name — changing the model automatically triggers a full re-index and cleans up stale collections.
5. **Reranker** — Optional separate agent/model for reranking similarity results.

## Chunking Strategies

| Strategy           | Description                                                                       |
| ------------------ | --------------------------------------------------------------------------------- |
| **auto** (default) | AST parsing for supported languages (tree-sitter), line-based for everything else |
| **ast**            | Always use tree-sitter AST parsing. Falls back to line-based if unsupported.      |
| **line**           | Always use line-based splitting                                                   |

AST chunking extracts function, method, and type declarations as individual chunks, preserving semantic boundaries.

Supported languages: Go, Python, TypeScript, TSX, JavaScript, JSX, Rust, Java, C (`.c`, `.h`), C++ (`.cc`, `.cpp`, `.hpp`).

### Defaults

| Setting              | Default         |
| -------------------- | --------------- |
| Strategy             | `auto`          |
| Max tokens per chunk | 500             |
| Overlap tokens       | 75              |
| Token estimation     | `len(text) / 4` |

## File Filtering

Indexed files are controlled by gitignore patterns in the config. These are combined with any `.gitignore` at the project root.

```yaml
gitignore: |
  *
  !*/
  !internal/
  !internal/**/*.go
```

## Reranking

When a reranker agent is configured, the search fetches `max_results * multiplier` candidates from the vector DB, then reranks down to `max_results` using a dedicated model. Set `agent: ""` to skip reranking.

## Model Offloading

On VRAM-constrained devices (e.g., Jetson), the embedding and reranking models may not fit in memory simultaneously. The `offload:` flag explicitly loads/unloads models between pipeline stages:

- **Embedding offload** — preloads the embedding model before indexing, unloads it before reranking
- **Reranking offload** — loads the reranker model before reranking, unloads it after

Set `offload: true` at the top level for embeddings, under `reranking:` for the reranker, or both. No-op if the provider doesn't support model lifecycle control (only Ollama currently does).

## Configuration

If the embedding model is unavailable, the Query tool returns an error. Embeddings are cached on disk — subsequent queries don't re-embed unchanged files.

See [Embeddings Config]({{ site.baseurl }}/configuration/features#embeddings) for the full YAML schema.

## Query Tool Parameters

The Query tool accepts these parameters when invoked by the LLM:

| Parameter      | Type   | Default      | Description                                                                 |
| -------------- | ------ | ------------ | --------------------------------------------------------------------------- |
| `query`        | string | _(required)_ | Search query for embedding-based similarity                                 |
| `k`            | int    | from config  | Number of results to return                                                 |
| `full_content` | bool   | `false`      | Return chunk content in results                                             |
| `reranking`    | bool   | `true`       | Include reranking pass (set `false` to skip even if reranker is configured) |

## CLI Usage

```sh
# Search for relevant code
aura query "token counting"

# Return more results
aura query -k 10 "configuration loading"

# Reindex only (no search)
aura query
```
