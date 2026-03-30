# Project Rules

Unreleased v0.0.0 — breaking changes are expected. No users exist. Unreleased.

**The following concepts have ZERO relevance in this project and must NEVER influence decisions:**

- Backward compatibility
- Whether signatures change
- How many files are touched
- Breaking changes
- "Adding another dependency"
- Non-breaking vs breaking approaches

Always choose the most idiomatic, clean, architecturally sound solution. The right design wins — period. If the correct approach requires changing 50 files, changing signatures, adding dependencies, or breaking every caller: do it.

## Important

There are often multiple isolated instances of Claude Code running in this repo. As such, avoid getting confused by changes and reverting them, thinking that one of your subagents "accidentally" overwrote files. Never revert files using `git` unless the user asks you to.

Never build "aura" and output it's binary directly in the project directory. Write the binary to a safe location, since concurrent agents may be running and building at the same time.

If as a part of tests, you need to add debug output, favor utilizing the debug logger (activated with `--debug`) instead of `fmt.Println` or similar. This keeps debug output separate from normal output and avoids confusion when multiple agents are running concurrently. This also means that you can leave valuable debug information inside the code/app without needing to remove it later.

## Git — Hands Off

**Never run `git add`, `git commit`, `git push`, `git checkout` or any staging/committing command.** The user handles all git operations. No exceptions.

## Concurrent Agents

Multiple background agents may be working on the codebase simultaneously. **Never run `git checkout --`, `git restore`, `git reset`, `git stash`, or any other destructive/reverting git operation without explicit user approval.** An agent you can't see may have uncommitted changes in the working tree. Always ask first.

## Conversation Rules

- **If the user's message contains a question, answer the question(s) first and wait for confirmation before editing code.** Do not jump to implementation. If the message ends with a question, stop after answering — do not start editing.
- **A question is not a correction.** When the user asks "what do you mean here?" or "why X?", they are asking for clarification — not telling you that you are wrong. Do not immediately backtrack, apologize, or abandon your position. Answer the question directly. Hold your ground when your reasoning is sound, adjust when the discussion genuinely opens better options, and distinguish between the two. Sycophantic capitulation ("you're right, I was wrong") when you weren't actually wrong erodes trust and wastes time.
- **When you discover something unexpected, flag it.** Do not silently fix issues, work around problems, or gloss over observations. State what you found, what it means, and ask if the user wants you to act on it. The user's awareness is more valuable than a quick fix.
- **Do not assume root causes — investigate or ask.** When something fails, do not guess why and move on. Either dig into the actual error (logs, debug output, server-side state) or ask the user for help. "Resource limitation" or "not reachable" are not acceptable conclusions without evidence. The user has access to server logs, tokens, and infrastructure you cannot see — ask them.
- **Push back on low-value work during active discussion.** When you are in a back-and-forth conversation with the user — discussing, refining, or fleshing out a task — and your honest assessment is that the proposed feature adds complexity without proportional benefit, say so briefly before diving into design. One sentence of "I'm not sure this pulls its weight because X" is expected. This is not a veto — the user decides — but silent compliance on dubious ideas wastes everyone's time. Design enthusiasm is not a substitute for judgment. **Scope: this applies ONLY during active discussion.** It does NOT apply when executing a fully-defined task, running auto-execution, or implementing work that has already been discussed and approved. Once a task is defined and queued, execute it without second-guessing.

## Behavioral Rules

- **When you hit a blocker, STOP and ask.** Do not silently work around it, rewrite code to avoid it, or spiral into increasingly desperate solutions. State the problem, explain what you tried, and ask the user what they want to do. The user's instructions take priority over your convenience — if an instruction can't be followed due to a technical limitation, that's a conversation, not a license to freestyle.
- **Never weaken a test to match a bug.** If a test fails because the code is wrong, fix the code — do not adjust the test to accept broken behavior. A test that catches a real bug is working correctly. Suggesting "adjust the test or fix the code" is not an acceptable framing — the answer is always fix the code.
- **Never patch vendored dependencies.** If a vendored library has a bug, the options are: upgrade it, fork it properly, or work around it at the call site. Editing files under `vendor/` is never acceptable.
- **Do it properly, not "safely."** Do not avoid touching files, changing signatures, or modifying callers just to minimize diff size. The right design wins over the small diff. If the correct approach requires updating 15 files, update 15 files. A hack that "avoids changes" is not safe — it's debt. This is an unreleased v0.0.0 project with zero users; there is nothing to protect.
- **Never build on top of buggy behavior.** If during investigation or implementation you discover that a dependency (internal or external) has a bug that affects your task, do NOT work around it or treat it as a given. Either fix the bug as part of your task (expanding scope), or file a dedicated task for it and block on the fix. Never proceed with an implementation that relies on broken behavior — the resulting code inherits the bug as a hidden assumption.
- **Exported/unexported is not a contract.** Change visibility freely. If something needs to be exported, export it. If something should be unexported, unexport it. There is no public API to break — this is v0.0.0. Do not treat an unexported symbol as a reason to add wrappers, accessors, or indirection instead of just making it public.

## Decision Authority

- **Never make design decisions autonomously.** Do not declare a feature "not viable", reject an approach, or close a design option based on your own judgment of performance, complexity, or feasibility. Present findings (data, measurements, trade-offs) and let the user decide. Statements like "X is unacceptable", "Y won't work", or "verdict: not viable" are off-limits unless the user explicitly asked for a recommendation.

## Hard Rules

- **Never reason about features or semantics based on current `.aura/config/` contents.** The config files are samples, not a spec. Users can write any valid config. "No existing config uses X" is never a justification for changing X's semantics, removing X, or treating X as dead code. Design for the full config surface area, not for what happens to be checked in today.
- No backward compatibility code, fallbacks, or migration shims. If something is replaced, delete the old version entirely.
- **No silent fallbacks.** If something is misconfigured, missing, or invalid — error loudly. Never silently degrade, pick a default, or guess what the user meant. A clear error message is always better than magic behavior that hides problems.
- No time estimates anywhere — not in docs, plans, tasks, or commit messages.
- No `golangci-lint` or `go vet` - rely on `go build` for correctness.
- **Only run `go build` after code changes.** Do not rebuild after modifying documentation, config, or other non-Go files.
- **Three tiers of testing — all required for every task:**
  1. **Go tests:** `go build ./...` AND `go test ./...`. If any existing test fails: (a) determine if the test is now outdated due to the new functionality — if so, update the test to match the new correct behavior; (b) if the test is still valid and the failure indicates a bug in the new code — fix the code, not the test.
  2. **Automated integration suites:** `tests/run.sh` (or relevant suites) and `tests/smoke.sh`. These exercise the full pipeline — config loading, tool execution, plugins, sessions, etc.
  3. **Interactive verification via web API:** Start `aura web` and use the JSON SSE endpoint to verify behavior interactively. This is the most precise way to test conversational features, mode switching, tool execution, and multi-turn behavior.

     ```bash
     # Start aura web (always specify --agent)
     aura --debug --agent high --env-file=secrets.env web --bind 127.0.0.1:9999 &
     # Listen to structured events
     curl -sN http://127.0.0.1:9999/events/json > /tmp/events.txt &
     # Send messages
     curl -s -X POST http://127.0.0.1:9999/send -d '{"text":"Hello"}'
     # Send slash commands
     curl -s -X POST http://127.0.0.1:9999/command -d '{"command":"mode edit"}'
     # Respond to ask/confirm dialogs
     curl -s -X POST http://127.0.0.1:9999/ask -d '{"answer":"yes"}'
     curl -s -X POST http://127.0.0.1:9999/confirm -d '{"action":"allow"}'
     # Read events from /tmp/events.txt to verify responses, tool calls, mode changes
     ```

     The JSON event stream includes: `status`, `message.added`, `message.started`, `message.update` (streaming deltas for content/thinking/tool), `message.finalized` (complete message with content, thinking, and tool call results), `assistant.done`, `command.result`, `ask`, `confirm`, `spinner`, `tool.output`, `synthetic`.

- **Run test suites one at a time.** Do NOT batch all suites in a single `tests/run.sh` call, a for-loop, a subagent, or a background task. The full suite takes 30-60 minutes with LLM suites and the output is too large to process. Instead: run one suite (`tests/run.sh 04-plugins`), read the result, fix failures, then run the next. Smoke suites (`tests/smoke.sh`) can be batched — they're deterministic and fast (~60s). See `tests/README.md` for details.
- **Never use line numbers in task definitions.** Line numbers drift as soon as any task lands. Identify code by function name, struct name, method name, or description (e.g., "in `ShouldCompact()` in `compact.go`" not "`compact.go:62`"). The implementer greps to find it.
- **Never store unbounded data in environment variables, command-line arguments, or any OS-limited channel.** Environment variables and CLI args share a kernel size limit (~2-3MB). Anything that grows with session length (tool history, conversation state, large JSON) MUST use files, pipes, or in-process memory — never env vars or arg lists. If you find yourself serializing a struct into an env var, stop and rethink.
- No `interface{}` — use `any`.
- No handrolled utilities — use established third-party libraries (e.g., `go-humanize`).
- Never describe this project as "production ready."
- When introducing new features/concepts or working on new `tasks` make sure to organize cleanly into firstly self-contained and clear packages in either `internal/` or `pkg/`, and secondly wiring in the new feature.
- Before creating or executing a task, verify the feature doesn't already exist. Push back if existing code already covers the request, or if the request is based on a misunderstanding of the current features, or does not offer a good UX improvement.
- Prefer public methods over private, unless private methods really can corrupt the data. For example <type>.Format(...) can be public, even if it's just used internally, if it is a type of method that "might" be useful to a caller.
- Do not build the app and output into the repository directory. Build to `/tmp/<task>` or another safe location - there are often multiple agents/tasks being executed concurrently, and they will clobber each other's output if they share a build directory.

## Structural Rules

- **New tools must embed `tool.Base`** — it provides no-op `Pre`/`Post`/`Paths`. Only override the methods that have real logic.
- **Tools have two execution paths.** Direct (in-process, no sandbox) and sandboxed (re-exec as child process with Landlock). When designing or modifying tool behavior, always consider BOTH paths. The sandboxed path re-execs the aura binary — the child process loads tools fresh, receives args as CLI JSON, and returns results as JSON on stdout. State that exists in the parent (context values, assistant fields, cached data) does NOT exist in the child. If a feature works in-process but breaks under sandbox re-exec, it's not done.
- **Refactoring is not find-and-replace.** Before substituting one implementation for another, understand the _semantic role_ of the code being changed — not just its output. Ask: "Is the current behavior correct, or is it a bug I'm about to cement?" A mechanical substitution that preserves output can silently make a latent bug permanent. If the existing behavior looks wrong, flag it.
- **Before changing anything, understand all callers and all execution paths.** Grep for every call site. Read the code that calls the code you're changing. A method called from 2 places with different assumptions will break in the path you didn't check. This applies to refactoring just as much as new features — "just moving code around" breaks things when the moved code had implicit contracts with its callers (e.g. `Execute()` being self-sufficient, Go zero values as implicit defaults, void methods becoming error-returning). If you can't enumerate every caller and explain why the change is safe for each one, you haven't finished reading.
- **Display method naming convention:**
  - `String()` — Go stringer, simple `%s` output
  - `Display()` — rich human-facing output (colors, padding, humanize)
  - `Render()` — reserved for conversation rendering (messages → text)
- **Config YAML uses map-keyed entries** — the entity name is always the map key, never a `name:` field. Multiple entries can live in one file. The loader is decoupled from filenames.
- **Prefer verbosity over abstraction** — a few repetitions of a short pattern is fine. Don't extract helpers, maps, or indirections just to deduplicate 3-5 lines repeated 2-3 times. Only consolidate when repetition is genuinely causing maintenance bugs.
- **New features must reuse existing pipelines.** When adding a feature that is conceptually a variant of an existing one, wire through the same code path — don't reimplement the pipeline with a subset of capabilities. Check what the closest existing feature already does and match it.
- **Before adding new fields or toggles, check if existing config already covers the behavior.** A `compaction.enabled` bool is unnecessary when `compaction.threshold: 1000` achieves the same thing. Don't add a new control surface when an existing one has the range to express it.
- **Before adding indirection (override fields, wrapper methods), check the data's ownership semantics.** If a struct holds config by value (not pointer), you can mutate it directly — no `fooOverride *int` + `foo()` accessor needed. Grep for how existing `Set*` methods work before inventing a new pattern.
- **Question every override level in a task.** CLI flag, task definition, agent frontmatter, global config — each level must have a concrete, distinct use case. "It might be useful" is not sufficient. If two levels would always be set together, or one subsumes the other, collapse them. Fewer override levels = less code, less debugging surface, less documentation.
- **When adding a new variant to a type system (enum, message type, tool type, provider), don't scatter case-by-case handling across call sites.** Add intent-based methods on the collection that encapsulate filtering, so consumers express what they need — not what to exclude. If adding a new variant requires updating 10 call sites, the abstraction is wrong.

## Go Conventions

- Prefer structs with methods over standalone functions.
- **Method naming — avoid `Get`/`Set` prefixes:**
  1. **Getters:** Drop `Get`. `Name()` not `GetName()`. For boolean state, use `Is`/`Has`/`Can` prefix: `IsRunning()` not `GetRunning()` or `Running()`.
  2. **Setters:** Try to reword to a natural verb first. `MarkRunning()` not `SetRunning()`, `Fail(err)` not `SetFailed(err)`, `UseReranker(p)` not `SetReranker(p)`.
  3. **Fall back to `Set` only when no natural verb fits** (e.g., `SetAuto(bool)` — no verb for toggling a flag).
- **Use `merge.Merge()` for all struct merging** (`internal/config/merge`). Never call `mergo.Merge` directly — the wrapper enforces canonical options (override + no-dereference + slice/map transformer).
- **Information Expert: put behavior on the type that owns the data.** If a function only operates on one type, it should be a method on that type — not a standalone function in a consuming package. This applies to helpers, formatters, safe-access wrappers, and conversions alike.

**Information Expert — BAD (standalone function in consumer):**

```go
// assistant package defines a helper for a type it doesn't own
func derefModel(m *model.Model) model.Model {
    if m == nil { return model.Model{} }
    return *m
}
// caller
derefModel(a.resolvedModel).ContextLength
```

**Information Expert — GOOD (method on owning type):**

```go
// model package — behavior lives on the type
func (m *Model) Deref() Model {
    if m == nil { return Model{} }
    return *m
}
// caller
a.resolvedModel.Deref().ContextLength
```

- Prefer adding formatting/display/logic/conversion methods on the domain struct that owns the data, rather than scattering that logic across consuming packages.
- **Corollary: never pass fields you don't own as bare parameters.** If a function needs data from another struct, either accept that struct directly or have the caller compose output from methods on the owning types. Don't create parameter-bag structs in the wrong package just to group someone else's fields.

**Rich domain types — BAD (formatting in consumer):**

```go
// In consuming package — formatting logic doesn't belong here
used := humanize.SIWithDigits(float64(status.TokensUsed), 1, "")
max := humanize.SIWithDigits(float64(status.TokensMax), 0, "")
statusParts = append(statusParts, fmt.Sprintf("tokens: %s/%s (%.0f%%)", used, max, status.TokensPercent))
```

**Rich domain types — BAD (parameter bag in wrong package):**

```go
// stats package defines a struct for data it doesn't own
type ContextSnapshot struct { ContextUsed, ContextMax int; ContextPercent float64 }
func (s *Stats) Display(ctx ContextSnapshot) string { ... }
```

**Rich domain types — GOOD (method on owning struct):**

```go
// Method on the struct that owns the data
func (s Status) TokensDisplay() string { ... }

// Consuming package just calls the method
statusParts = append(statusParts, status.TokensDisplay())
```

**Rich domain types — GOOD (caller composes from owning methods):**

```go
// Each struct formats what it owns; caller composes
output := stats.Display()          // formats session metrics
output += "\n" + status.ContextDisplay(msgCount) // formats context (owned by Status)
```

- **Error wrapping: avoid stuttering and noise.** Before wrapping, check what callers already add — don't produce `"fetching models: listing models: connection refused"`. No caller's arguments (they already have them), no info already in the inner error (OS errors include paths), no noise words ("failed", "error", "could not").
- **Prefer nested structs over flat prefixed fields.** When multiple fields share a logical prefix (e.g., `guardrailToolCallsAgent`, `guardrailUserMessagesAgent`), group them under a struct. Flat fields are fine for singletons (`compactAgent`), but the moment a concept has 2+ related fields, introduce a grouping struct. `a.guardrail.toolCalls` reads better than `a.guardrailToolCallsAgent` and scales without stuttering.
- **Use `heredoc.Doc()` for all multiline string literals** — descriptions, usage text, prompts, templates. Never use raw backtick strings with manual indentation.
- Ollama URL: `http://host.docker.internal:11434` (not `localhost`). -> if not accessible, ask user to start it. -> leverage the `ollama` mcp if you need logs from ollama
- llamacpp URL: `http://host.docker.internal:8081` (not `localhost`). -> if not accessible, ask user to start it.
- Non-local providers (anything other than `ollama` and `llamacpp`) require API tokens. Tokens live in `secrets.env` at the project root (not tracked by git). When runtime-testing with non-local providers, pass `--env-file=secrets.env`. Without it, you'll get 401/403 auth errors.
- Adding dependencies: `go get -u <package>` then `go mod vendor`.
- After any dependency change: `go mod tidy; go mod vendor`.

## CLI Flag Scoping

**All root-level flags (`--dry`, `--debug`, `--agent`, `--mode`, `--model`, `--provider`, `--include-tools`, `--exclude-tools`, `--output`, etc.) MUST appear before the subcommand.** They are local to the root command and urfave/cli will error out if they are placed after the subcommand.

```
# Correct:
aura --dry=render run "prompt"
aura --debug --agent=high run "Hello"
aura --model qwen3:32b run "explain this"

# Wrong — flags silently ignored:
aura run --dry=render "prompt"
aura run --debug "Hello"
```

## Testing

- **Test tools/features through the assistant path**: `aura run "/tools"`, `aura run "/some command"` — this exercises the full pipeline (agent/mode/task filtering, opt-in, etc.)
- **`aura tools`** only applies global filters — it does NOT apply agent/mode/task/opt-in filtering. Use it for tool inspection, not for verifying filtering behavior.
- **Default agent has no tools.** The `Base` agent uses mode `Ask` (no tools). When testing tool execution, plugin hooks on tool events, or anything beyond simple Q&A, you MUST use `--agent=high` (or another agent with tools enabled). Without this, the test silently skips the code path you're trying to verify.
- **`aura run` with prompt arguments is non-interactive.** It processes the given prompts and exits — no TUI, no stdin needed. Do NOT add `< /dev/null`, `2>&1 | head`, or other redirections. Use `--output <file>` to capture output. Use `--debug` to get debug log entries in `.aura/debug.log`.

  ```
  # Build to /tmp (never build into project dir — concurrent agents may clobber it):
  go build -o /tmp/aura-build/aura .
  # Correct (--agent=high for tool-using tests):
  /tmp/aura-build/aura --debug --agent=high --output /tmp/test/out.txt run "Hello" "Read file X"
  # Correct (default agent is fine for ask-only tests):
  /tmp/aura-build/aura --debug --output /tmp/test/out.txt run "Say hello"
  # Wrong — breaks the process:
  ./aura run ... < /dev/null 2>&1 | head -60
  ```

- **Think creatively when testing.** Prefer the Ollama provider (free, always running) over mocks or fixtures. Use `--include-tools` to constrain the LLM to exactly the tool you want tested — this makes LLM-based tests practically deterministic. Example: to verify a plugin's `BeforeToolExecution` hook rewrites Bash commands:

  ```
  /tmp/aura-build/aura --debug --agent=high --include-tools Bash \
      --output /tmp/test/out.txt \
      run "Use the Bash tool to run: echo hello"
  ```

  The agent has no choice but to call Bash, the hook fires, and you inspect the output.

## Context

- `AURA.md` at root is the comprehensive project reference (architecture, features, pain points, legacy comparison).
- `tasks/INDEX.md` lists all pending work ordered by difficulty.
- `docs/` contains aura's user-facing documentation
- Ignore the `./assets` directory — it is not relevant to you.
