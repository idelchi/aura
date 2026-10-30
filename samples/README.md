# Aura starter configuration

`aura init` installs this reduced version of the canonical development configuration. It shares the agentic prompts,
read-only Ask/Plan modes, Edit mode, Go hooks, plugin settings and compaction policy. It omits Garfield endpoints,
Wayfinder identities, infrastructure MCP definitions and operational tasks.

The default `Qwen` agent uses Ollama at `http://localhost:11434` and the `qwen3.5:9b-q8_0` model. Install that model on your
server or change the preset to an available model. `LlamaCpp` is the equivalent direct-server example at port 8080;
its model name must match the alias served by your worker. When Aura runs in a container and these services run on
the host, change the URLs to the hostname that reaches that host, such as `host.docker.internal`.

OpenAI, Anthropic, Google, OpenRouter, Copilot and Codex presets are included. API keys use
`AURA_PROVIDERS_<PROVIDER>_TOKEN`; Copilot and Codex use `aura login`. `api: responses` selects the OpenAI wire API
explicitly, including for models newer than the SDK's built-in name list.

Install the recommended plugin sets after initializing:

```bash
aura plugins add --subpath plugins/injectors,plugins/tools --ref dev https://github.com/idelchi/aura
```

The sample does not start or install optional model services. To use these helpers, configure the corresponding
provider and model aliases to match services you run:

| Feature | Sample provider | Address / model |
| --- | --- | --- |
| Speech | `kokoro` | Port 8081; model `kokoro`, voice `af_bella` |
| Transcription | `whisper` | Port 8082; model `whisper-1` |
| Embeddings | `embeddings` | Port 8083; Qwen3 Embedding 0.6B |
| Reranking | `reranker` | Port 8084; Qwen3 Reranker 0.6B |
| Vision | `ollama` | Model `qwen3-vl:2b-instruct-q4_K_M` |

Go hooks require gopls and golangci-lint. Pyright is disabled until explicitly enabled. Edit the LSP and hook files
to match your installed toolchain. Linux filesystem sandbox settings do not provide Windows Landlock enforcement.
No MCP server is enabled by the portable starter; examples show how to add one.

Files named `.example.*` document configuration fields and are not loaded. The starter supplies configuration only:
models, provider accounts, optional services and installed plugins remain environment-owned state.

## Agent file conventions

Start agent frontmatter with `name` and `description`, then keep inheritance, defaults, model, thinking, prompt/mode,
tools, hooks, features and visibility fields in that order when present. Capitalize agent identifiers consistently
(e.g. `Flash`, `Codex-Sol`, `Gemma-E2B`) and preserve model/provider IDs verbatim. Describe the actual model and role,
with meaningful settings where useful. Avoid catalogue, billing or generic alternative wording. Agent names are
lookup keys: update inheritance, feature and task references when renaming them.
