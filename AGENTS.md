# Project Guidance

Use `CLAUDE.md` as the broader project reference. This file captures the rules and architecture notes most likely to prevent drift for Codex-style agents.

## Working Rules

- This is unreleased `v0.0.0`. Prefer the clean architecture over compatibility shims, small diffs, or preserving old signatures.
- Multiple agents may work in this repo at once. Never revert, reset, checkout, stash, or discard changes you did not intentionally make unless the user explicitly asks.
- Do not stage or commit unless the user explicitly asks in the current turn.
- Build binaries outside the repo, for example `/tmp/aura-build/aura`.
- Use `/usr/local/go/bin/go` for Go commands.
- Do not use `golangci-lint` or `go vet` unless the user explicitly asks.
- Use `go build ./...` and `go test ./...` for Go validation after code changes. Run focused tests first when the affected package is clear.
- Do not treat checked-in `.aura/config/` samples as the semantic spec. Users can write any valid config.

## Aura Architecture Notes

- Config loading lives in `internal/config`. Files are loaded, defaults are applied, then `Config.Validate` and `ValidateCrossRefs` run.
- Feature overlays are resolved later with `Config.ResolveFeatures`: global features, then agent features, then mode features.
- CLI `--max-steps`, `--token-budget`, and `--override` are intentionally converted to override nodes and applied even later in `Assistant.rebuildState`, after all feature merges. This keeps CLI overrides final.
- Any semantic validation that must also cover CLI overrides belongs on the owning config/domain type and must run after `overrideNodes.Apply` in `rebuildState`. Avoid parsing override strings in the CLI layer for domain validation.
- `--dry=render` can otherwise miss late override behavior, so changes to late overrides should consider the pre-first-rebuild path.
- Scalar zero has two different meanings depending on path:
  - Normal feature merge/default paths often use zero as "unset" because scalar zero does not override in `merge.Merge`.
  - CLI override nodes can explicitly write zero through YAML. Domain validation must decide whether zero is valid for that field.
- Plugin source under `plugins/` is tracked source. Installed local plugin copies under `.aura/plugins/` are ignored runtime copies. If live-testing an in-repo plugin through Aura, update or reinstall the active copy too, but commit the source copy.

## Max Steps Semantics

- `features.tools.max_steps` is the number of normal LLM loop iterations allowed before wrap-up.
- The assistant loop increments `a.loop.iteration` before `BeforeChat` hooks run.
- The loop hard-stops after `max_steps + 1`, reserving one final text-only response after tools are disabled.
- Therefore the `max-steps` injector should disable tools when `iteration > max_steps`, not when `iteration == max_steps`.
- `max_steps <= 0` is invalid resolved config. `token_budget == 0` is valid and means disabled.

## Implementation Habits

- Before changing a pipeline, trace all execution paths: normal run, dry render, noop, plugin hooks, MCP rebuild, resume, and task/subagent paths if relevant.
- Put behavior on the type that owns the data. Config validation should live in config/domain types, not in CLI or assistant consumers.
- New features should reuse existing pipelines instead of adding parallel mechanisms.
- Prefer direct, explicit code over clever helper abstractions unless duplication is causing real bugs.
