---
layout: default
title: Sandboxing
parent: Features
nav_order: 7
---

# Landlock Sandboxing

Aura uses Linux Landlock LSM to restrict filesystem access during tool execution.

## How It Works

When sandboxing is enabled, Aura re-executes itself with Landlock filesystem restrictions applied. Only tools that declare `Sandboxable() true` are affected — other tools (todos, vision, transcribe, speak, query) run without restrictions.

The parent process pipes runtime context (`sdk.Context`) to the child as JSON via `exec.Cmd.Stdin`. This delivers agent name, model info, token state, stats, and workdir to the sandboxed process — the same context that in-process tools receive directly. Tool arguments remain CLI arguments (small, bounded); context bypasses `execve()` entirely via the kernel pipe (no ARG_MAX limit).

## Configuration

**File:** `.aura/config/sandbox/sandbox.yaml`

```yaml
sandbox:
  enabled: true

  restrictions:
    # Read-only paths
    ro:
      - /bin
      - /etc
      - /home
      - /lib
      - /lib64
      - /opt
      - /run
      - /sbin
      - /tmp
      - /usr
      - /var

    # Read-write paths
    rw:
      - /tmp
      - $HOME/.cache
      - $HOME/go
      - /dev/null
```

Paths support environment variable expansion (`$HOME`, etc.).

## Mode and Agent Overrides

Modes and agents can extend or override sandbox restrictions via their `features:` frontmatter. Sandbox config follows the same override semantics as all other features.

Use `extra` to **add** paths on top of whatever the base restrictions resolved to:

```yaml
features:
  sandbox:
    extra:
      rw:
        - .
```

Use `restrictions` to **replace** the base paths entirely:

```yaml
features:
  sandbox:
    restrictions:
      rw: [/tmp, /var/log]
```

Other sandbox fields can also be overridden:

```yaml
features:
  sandbox:
    enabled: false # explicitly disable sandbox for this mode/agent
```

Feature merge order: **Global → Agent → Mode → Task = Effective Features**. All layers use the same override merge — non-nil replaces, nil inherits.

## Requested vs Enabled

Sandbox state is split into two independent flags:

| State         | Meaning                                                                                                             |
| ------------- | ------------------------------------------------------------------------------------------------------------------- |
| **Requested** | The user wants sandboxing (set via config `enabled: true`, `/sandbox`, or `Ctrl+S`). Independent of kernel support. |
| **Enabled**   | Landlock is actually enforcing restrictions. True only when both requested AND the kernel supports Landlock.        |

Landlock requires Linux 6.2+. On older kernels or non-Linux systems, sandboxing is requested but not enforced — tools run without filesystem restrictions.

A user on a system without Landlock support can still request sandbox — the UI shows the distinction (lock vs unlock emoji), and templates can branch on either flag. `/info` and `/sandbox` display both values.

Both states are available in prompt templates as {% raw %}`{{ .Sandbox.Requested }}`{% endraw %} and {% raw %}`{{ .Sandbox.Enabled }}`{% endraw %}. The {% raw %}`{{ .Sandbox.Display }}`{% endraw %} field provides a pre-rendered restriction summary for prompt injection.

## Controls

| Method                    | Description                                           |
| ------------------------- | ----------------------------------------------------- |
| `/sandbox` or `/landlock` | Show or toggle sandbox                                |
| `Ctrl+S`                  | Toggle sandbox on/off                                 |
| Status bar                | Shows lock emoji (enabled) or unlock emoji (disabled) |

## CLI Sandboxing

The `aura tools` command supports sandboxed tool execution:

```sh
# Run with read-only sandbox
aura tools --ro Read '{"path": "file.txt"}'

# Add custom paths (repeat the flag for multiple paths)
aura tools --ro-paths /extra/path --ro-paths /another --rw-paths /tmp/work Read '{"path": "file.txt"}'
```
