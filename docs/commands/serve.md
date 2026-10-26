---
layout: default
title: aura serve
parent: Commands
nav_order: 4
---

# aura serve

Serve the automation API (`/v1`) for one session.

```sh
AURA_SERVE_TOKEN=<at least 32 characters> aura [root flags] [--resume <id>] serve --ready-file <path> [flags]
```

| Flag           | Default       | Description                                           |
| -------------- | ------------- | ----------------------------------------------------- |
| `--bind`       | `127.0.0.1:0` | Address to listen on (port 0 picks a free port)       |
| `--ready-file` | (required)    | File receiving the server address or startup error    |

Without `--resume`, a fresh session is created and saved before the server announces it. Root flags (`--config`,
`--agent`, `--model`, …) apply as for any session; they must precede `serve`.

```sh
export AURA_SERVE_TOKEN=$(openssl rand -hex 32)
aura --home "" --config .aura --config /run/aura-home --agent Flash --resume task-1 \
  serve --ready-file /run/task-1.ready.json &
jq -r .url /run/task-1.ready.json
curl -s -H "Authorization: Bearer ${AURA_SERVE_TOKEN}" "$(jq -r .url /run/task-1.ready.json)/v1/server"
```

The token is removed from the environment before any tool or MCP server starts. The ready file holds either
`{"api":1,"url",…}` or `{"api":1,"error":{…}}` (the process then exits 1). SIGINT, SIGTERM and `POST /v1/shutdown`
settle the active run before exiting.

The complete contract — endpoints, run states, outcomes, error codes, events and the Python client — is in
[Automation]({{ site.baseurl }}/features/automation).
