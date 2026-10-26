---
layout: default
title: aura session
parent: Commands
nav_order: 5
---

# aura session

Inspect, import, export, fork and freeze stored sessions without starting a model.

```sh
aura [root flags] session list                                   [--json]
aura [root flags] session show   <id>                             [--json]
aura [root flags] session import <file> [--id <id>] [--freeze]    [--json]
aura [root flags] session export <id> --output <file>             [--json]
aura [root flags] session fork   <parent> --id <child> [--title <t>] [--expect-revision <sha256>] [--json]
aura [root flags] session freeze <id> [--expect-revision <sha256>] [--json]
```

The store is `<write home>/sessions` — the last `--config` directory. Operations are idempotent; `--json` prints one
JSON object (errors as a JSON envelope on stderr, exit 1).

```sh
# Keep a reviewed conversation immutable and start independent tasks from it
aura session import baseline/session.json --freeze --json
aura session fork a5557ae1-ad1e-4f12-b845-f6f2172d2cb2 --id task-1 --json
aura --resume task-1 run "Continue with the assignment"
```

See [Sessions]({{ site.baseurl }}/features/sessions#store-operations) and the
[automation contract]({{ site.baseurl }}/features/automation#session-store).
