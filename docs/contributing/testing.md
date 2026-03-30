---
layout: default
title: Testing
parent: Contributing
nav_order: 3
---

# Testing

Aura uses a three-tier testing strategy. All tiers must pass for every change.

## Build

Always build to a temporary directory — never into the project root (concurrent agents may clobber each other):

```bash
go build -o /tmp/aura-build/aura .
```

## Tier 1: Go Tests

```bash
go build ./... && go test ./...
```

If an existing test fails after your change:

- If the test is **outdated** (your change intentionally alters behavior) — update the test
- If the test is **valid** (it caught a bug in your code) — fix the code, not the test

## Tier 2: Smoke Suites

31 deterministic suites that need no LLM. Run in ~60 seconds:

```bash
tests/smoke.sh
```

These use `--dry=render` (print resolved config, no LLM call) and `--dry=noop` (full pipeline with noop provider). They verify config loading, agent resolution, template rendering, tool filtering, and CLI flags.

## Tier 3: Integration Suites

55 individual suites (31 deterministic + 24 requiring Ollama). Run one at a time:

```bash
tests/run.sh 04-plugins    # run a specific suite
tests/run.sh               # run ALL suites (takes 30-60 minutes with LLM suites)
```

Never batch all suites in a single call or for-loop. The full run takes too long and output is too large to process. Run one, read the result, fix failures, then run the next.

See `tests/README.md` for the complete suite table and per-suite documentation.

## Writing a Smoke Suite

```bash
#!/usr/bin/env bash
suite_header "XX-name: Description"
setup_env "unique-name" "optional-overlay"

OUT="/tmp/aura-tests/xx-01.txt"
run_aura --dry=render --agent=Test --output "$OUT" run "test prompt"
assert_file_contains "test name" "$OUT" "expected pattern"
assert_file_not_contains "test name" "$OUT" "unwanted pattern"

suite_footer "XX-name"
```

`setup_env` creates an isolated test directory at `/tmp/aura-tests/<name>/.aura`, copying base fixtures and applying overlay layers.

## Assertion Helpers

| Helper                                                     | Purpose                     |
| ---------------------------------------------------------- | --------------------------- |
| `pass <name>`                                              | Record a passing test       |
| `fail <name> <message>`                                    | Record a failing test       |
| `skip <name> <reason>`                                     | Skip a test                 |
| `finding <severity> <message>`                             | Log an observation          |
| `assert_file_exists <name> <path>`                         | File existence              |
| `assert_file_not_exists <name> <path>`                     | File absence                |
| `assert_file_not_empty <name> <path>`                      | Non-empty file              |
| `assert_file_contains <name> <path> <pattern>`             | Grep for pattern            |
| `assert_file_not_contains <name> <path> <pattern>`         | Pattern absent              |
| `assert_file_contains_count <name> <path> <pattern> <min>` | Minimum occurrences         |
| `assert_json <name> <file> <jq> <pattern>`                 | jq path matches pattern     |
| `assert_json_gt <name> <file> <jq> <min>`                  | jq value above threshold    |
| `assert_exit_code <name> <expected> <actual>`              | Exit status check           |
| `assert_cmd_succeeds <name> <cmd...>`                      | Command succeeds            |
| `assert_aura_ok <name> <args...>`                          | Run aura and assert success |

## Execution Helpers

| Helper                                     | Purpose                                                  |
| ------------------------------------------ | -------------------------------------------------------- |
| `run_aura <flags...>`                      | Run `$AURA_BIN` with `--workdir` set to current test env |
| `run_aura_capture <var> <flags...>`        | Run aura and capture output to variable                  |
| `run_aura_bg <flags...>`                   | Run aura in background                                   |
| `show_aura <flags...>`                     | Run aura and display output (verbose)                    |
| `grep_show <pattern> <file>`               | Grep and display matches                                 |
| `setup_env <name> [overlays...]`           | Create isolated test env with fixtures                   |
| `install_plugin <env> <category> <source>` | Install test plugin into env                             |

## Fixtures and Test Plugins

**10 fixture overlays** in `tests/fixtures/`: `base`, `auto-test`, `auto-mode`, `compaction`, `custom-commands`, `hooks`, `inheritance`, `tasks`, `tool-policy`, `template-composition`. Each provides a `.aura/` config directory that merges on top of base.

**13 test plugins** in `tests/plugins/`: hook interceptors (before/after/block), tool overrides (noop-write/noop-patch), event observers (canary, compact-observer, agent-observer), request/response modifiers, and error handlers.

## Interactive Verification

For conversational features, mode switching, and multi-turn behavior, use the web API:

```bash
# Start aura web
/tmp/aura-build/aura --debug --agent high --env-file=secrets.env web --bind 127.0.0.1:9999 &

# Listen to structured events
curl -sN http://127.0.0.1:9999/events/json > /tmp/events.txt &

# Send messages
curl -s -X POST http://127.0.0.1:9999/send -d '{"text":"Hello"}'

# Send slash commands
curl -s -X POST http://127.0.0.1:9999/command -d '{"command":"mode edit"}'

# Respond to ask/confirm dialogs
curl -s -X POST http://127.0.0.1:9999/ask -d '{"answer":"yes"}'
curl -s -X POST http://127.0.0.1:9999/confirm -d '{"action":"allow"}'
```

JSON event types: `status`, `display_hints`, `message.added`, `message.started`, `message.update`, `message.finalized`, `assistant.done`, `command.result`, `ask`, `confirm`, `spinner`, `tool.output`, `synthetic`.

## Tips

- **`--dry=render`** — deterministic, no LLM. Tests template rendering and config assembly
- **`--dry=noop`** — full pipeline with noop provider. Tests tool execution without LLM
- **`--include-tools Tool1`** — constrain LLM to specific tools for near-deterministic behavior
- **`--debug`** — writes detailed logs to `.aura/debug.log`
- **`--output <file>`** — captures all output for inspection
- **`--agent=high`** — required for tool-using tests (default agent has no tools)
- **`--env-file=secrets.env`** — required for non-local providers (otherwise 401/403)
- **Slash command errors go to stderr**, not `--output`. Use `assert_exit_code` and debug log assertions
- **Rebuild before running suites** — `tests/run.sh` uses `/tmp/aura-build/aura`. Stale binary = false results
