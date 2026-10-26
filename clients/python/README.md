# aura-automation

Python client for Aura's machine contract (API version 1):

- `Store` — offline session store operations through `aura session … --json`
  (list, show, import, export, fork, freeze).
- `Server` — launch or attach to `aura serve` (one process per live session).
- `Client` — the `/v1` HTTP API: submit runs (idempotent by `request_id`), cancel,
  answer inputs, read the durable journal (long-poll or SSE) and wait for settlement.

Every response is validated by strict pydantic models (unknown fields fail), so a
contract drift between this client and the `aura` binary fails loudly. Pin both to the
same Aura commit. The normative contract is `docs/features/automation.md` in Aura.

```python
from pathlib import Path
from aura_automation import Aura, Store, Server

aura = Aura(binary=Path("/opt/aura"), args=["--home", "", "--config", ".aura", "--config", "/run/aura-home"],
            cwd=Path("/work"), env={"PATH": "/usr/bin"})
Store(aura).fork("baseline", "task-1")
handle = Server.launch(aura, ready_file=Path("/run/ready.json"), token=token, resume="task-1")
client = handle.client()
run = client.submit("task-1:author:1", prompt=assignment)
final = client.wait(run.id, on_event=mirror)
assert final.state == "persisted" and final.outcome == "completed"
handle.stop()
```

Prompts travel in HTTP request bodies, never in process arguments.
