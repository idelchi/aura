# tests/

Testing and reference documentation for Aura. Split into two tiers:

1. **Automated tests** — Bash test suites run via `run.sh`. This is the primary regression safety net.
2. **Manual guides** (`guides/`) — reference docs for features that require interactive testing, external servers, or visual inspection. Only covers what automated tests cannot.

## Running Automated Tests

```sh
# Build first (to /tmp to avoid clobbering project dir)
go build -o /tmp/aura-build/aura .

# All suites (including Ollama-dependent checks)
tests/run.sh

# Smoke test (deterministic suites, no LLM needed)
tests/smoke.sh

# Specific suites
tests/run.sh 01-config 03-hooks 17-templates
```

Requires: `aura` binary, Ollama running with `gpt-oss:20b` (for LLM suites).
Smoke suites need only `--dry=render` / `--dry=noop` / direct tool execution — no model.

The runner passes `--home "" --config <suite-dir>/.aura` to each helper invocation.
This disables the developer's global configuration and loads only that suite's fixtures and explicit overlays.
Default-agent selection remains active, so suites can test it. Fixture writes are visible to subsequent processes
without a filesystem-wide `sync`.

**Run suites one at a time.** Do NOT batch all suites in a single command or for-loop
and wait for the result — process each suite's results before starting the next. Instead:

1. Run one suite: `tests/run.sh 04-plugins`
2. Read the result
3. Fix any failures
4. Run the next suite

Smoke suites (no LLM) can be run together via `smoke.sh`. LLM suites must be
run individually.

Do NOT use subagents, bash for-loops, or background tasks to batch LLM suite runs.
The feedback loop is too slow and output gets lost or truncated.

## Test Suites

| Suite                      | What it covers                                                                                                                                                         | LLM? |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- |
| 01-config                  | dry-render, agent/mode switch, --show, --set, debug.log, tool filtering                                                                                                | No   |
| 02-tools                   | Read, Write, Glob, Rg, Bash, Ls, Mkdir, Patch (direct execution)                                                                                                       | No   |
| 03-hooks                   | post-hook firing, JSON stdin, file glob filtering, pre-hook blocking, DAG ordering                                                                                     | Yes  |
| 04-plugins                 | canary injector events, noop tool override, marker file logging                                                                                                        | Yes  |
| 05-thinking                | think levels, strip/rewrite on non-thinking model (noop), session JSON                                                                                                 | Yes  |
| 06-sessions                | /save, JSON structure, /export roles, --continue resume                                                                                                                | Yes  |
| 07-compaction              | ShouldCompact threshold, history split, /compact manual trigger                                                                                                        | Yes  |
| 08-debug                   | timestamp format, log categories, model/messages/tools in [chat]                                                                                                       | Yes  |
| 09-modes                   | ReadOnly tool limits, Restricted excludes Bash, mode switching, noop tool count                                                                                        | No   |
| 10-tasks                   | tasks list/show/run, foreach with shell items, timeout enforcement                                                                                                     | Yes  |
| 11-maxsteps                | hard stop, iteration count, Done tool opt-in                                                                                                                           | Yes  |
| 12-estimation              | rough, tiktoken, rough+tiktoken, native estimation methods                                                                                                             | Yes  |
| 13-inheritance             | child/grandchild inherit provider/model, tool overrides, mode inheritance                                                                                              | No   |
| 14-commands                | custom /greet, /testcmd, /tools, /info, /stats                                                                                                                         | Yes  |
| 15-sandbox                 | write to /tmp succeeds, write to /etc blocked, read from /etc allowed                                                                                                  | Yes  |
| 16-guardrails              | guardrail config modes (log/block/disabled), agent resolution, prompt mode                                                                                             | No   |
| 17-templates               | --set variables, .Agent/.Mode/.Provider template expansion, dry-render header                                                                                          | No   |
| 18-workspace               | AGENTS.md discovery, injection, agentsmd filtering (none/local/all)                                                                                                    | No   |
| 19-optin                   | tool count per mode, --include/--exclude-tools, agent tools.disabled, stacking                                                                                         | No   |
| 20-directives              | @File, @Bash, @Path expansion, missing file warning, directory listing                                                                                                 | No   |
| 21-agent-features          | hide, default, subagent, files autoload, feature overrides, AGENTS.md blocking                                                                                         | No   |
| 22-cli-flags               | --show, --print, --print-env, --workdir, --env-file, --model, --think, init                                                                                            | No   |
| 23-tasks-advanced          | tasks list/show detail, foreach/finally/on_max_steps config, inheritance                                                                                               | No   |
| 24-mcp-config              | aura mcp, server config (stdio/http), disabled servers, --include/--exclude-mcps                                                                                       | No   |
| 25-hooks-advanced          | silent, disabled, inheritance, depends, file glob, timeout, pre/post events                                                                                            | No   |
| 26-approval-rules          | tool_auto patterns, wildcard rules, policy merge, empty rules                                                                                                          | No   |
| 27-commands-advanced       | custom command loading, argument substitution, hints, /tools, /info                                                                                                    | No   |
| 28-tool-policy             | deny/confirm/auto tiers, policy in prompt, combined filter+policy modes                                                                                                | No   |
| 29-error-paths             | invalid agent/mode/provider, circular inheritance, bad YAML, double default                                                                                            | No   |
| 30-config-layering         | overlay agents/hooks/modes/commands, multiple overlays, last-wins, empty overlay                                                                                       | No   |
| 31-optin-wildcard          | opt_in wildcard immunity, explicit override, --include/--exclude with opt_in                                                                                           | No   |
| 32-auto-mode               | /auto iterations, /done + Done tool, max_steps hard stop, done-reminder plugin                                                                                         | Yes  |
| 33-media                   | vision/TTS/STT config, aura vision/speak/transcribe, aura query (jetson providers)                                                                                     | Yes  |
| 34-reload-resume           | /reload config rebuild, /save + --continue resume, context retention                                                                                                   | Yes  |
| 35-subagent                | subagent config, max_steps, default_agent, Task tool delegation                                                                                                        | Yes  |
| 36-multi-prompt            | multi-prompt processing, slash + prompt combos, empty prompt handling                                                                                                  | No   |
| 37-task-env                | task env vars (env:, env_file:, precedence, isolation)                                                                                                                 | No   |
| 38-hot-reload              | per-turn config reload, /reload command, tools rebuilt per turn                                                                                                        | No   |
| 39-slash-commands          | /ctx, /policy, /hooks, /plugins, /skills, /verbose, /sandbox, /agent, /mode                                                                                            | No   |
| 40-plugin-override         | Write/Patch plugin override intercept, marker files, coexistence                                                                                                       | Yes  |
| 41-bash-rewrite            | Bash tool command rewrite templates, Sprig functions                                                                                                                   | No   |
| 42-plugin-interception     | BeforeToolExecution arg rewrite, AfterToolExecution output modification                                                                                                | Yes  |
| 43-multivalue-flags        | --config/--set/--include-tools multi-value accumulation                                                                                                                | No   |
| 44-deferred-tools          | Deferred tool index in system prompt, tag presence/absence                                                                                                             | No   |
| 45-cli-subcommands         | skills list/show, plugins list/show, error handling                                                                                                                    | No   |
| 46-input-guard             | Input size guard rejection, acceptance, default percentage                                                                                                             | Yes  |
| 47-slash-commands-extended | /set, /window, /readbefore, /exit, /todo, /auto, /done, /think, /reload, /mcp, /insert, /name, /clear, /drop, /save, /export, /undo, /fork, /resume, /compact, /replay | No   |
| 48-sdk-response-gate       | AfterResponse SDK enrichment (skip, replace, read)                                                                                                                     | Yes  |
| 49-sdk-error-handler       | OnError SDK enrichment (classification, skip, retry)                                                                                                                   | Yes  |
| 50-sdk-compact-observer    | AfterCompaction SDK timing (success/failure observation)                                                                                                               | No   |
| 51-sdk-agent-observer      | OnAgentSwitch SDK timing (previous/new/reason observation)                                                                                                             | No   |
| 52-sdk-request-modifier    | BeforeChat RequestModification (append system, skip)                                                                                                                   | Yes  |
| 53-bash-condition          | bash: condition evaluation, shlex argument splitting, negation                                                                                                         | No   |
| 54-until-command           | /until looping command: exit conditions, --max cap, negation, debug logging                                                                                            | No   |
| 55-template-composition    | Template composition: DAG validation, include directives, agent/mode/file composition                                                                                  | No   |

## Test Infrastructure

```
tests/
  run.sh               Test runner with assertion framework
  smoke.sh             Deterministic smoke suites (no LLM)
  suites/              Individual test suites (01-55)
  plugins/
    canary/            Injector that logs all events to JSONL
    noop-write/        Overrides Write tool (logs but doesn't write)
    noop-patch/        Overrides Patch tool (logs but doesn't patch)
    response-gate/     AfterResponse hook (skip/replace/read response)
    error-handler/     OnError hook (classify, retry, skip errors)
    compact-observer/  AfterCompaction hook (log compaction events to JSONL)
    agent-observer/    OnAgentSwitch hook (log agent switch events to JSONL)
    intercept-before/  BeforeToolExecution hook (argument rewriting)
    intercept-block/   BeforeToolExecution hook (tool blocking)
    intercept-after/   AfterToolExecution hook (output modification)
    intercept-after-error/ AfterToolExecution hook (error path)
    context-echo/      BeforeChat hook (echo SDK context to file)
    request-modifier/  BeforeChat hook (request modification — append system, skip)
  fixtures/
    base/              Minimal Ollama config (provider, agents, modes, prompts)
    auto-test/         Test agent variants (thinking, strip, rewrite, hidden)
    auto-mode/         Low max_steps for auto-mode tests
    compaction/        Aggressive compaction thresholds
    custom-commands/   Custom slash commands
    hooks/             Pre/post hook configs
    inheritance/       Parent/child agents, inherited prompts
    tasks/             Task definitions (hello, foreach, shell, env)
    tool-policy/       Restricted mode with deny/confirm/auto tiers
  guides/              Manual testing reference docs (flat)
```

## Manual Guides

Reference docs for features that need interactive testing or external infrastructure.
These do NOT duplicate what automated tests cover — they only document what can't be automated.

| Guide                              | Covers                                                               |
| ---------------------------------- | -------------------------------------------------------------------- |
| `agents-modes/agents-modes.md`     | Subagent delegation, prompt assembly, mode sandbox extensions        |
| `cli/cli.md`                       | OAuth flow, /assert conditions, MCP tool execution, --raw JSON       |
| `cli/tasks.md`                     | Task runtime behavior, foreach execution, template delimiters        |
| `config/config.md`                 | Hot reload (/reload), template system ({{ }} vs $[[]])               |
| `core/auto-mode.md`                | /auto toggle, done-reminder injector                                 |
| `guardrails/guardrails.md`         | Runtime guardrail behavior, response protocol                        |
| `hooks/user-hooks.md`              | Runtime hook behavior, $FILE population, mvdan/sh                    |
| `input/directives.md`              | @Image multimodal, input size guard                                  |
| `mcp/servers.md`                   | Runtime MCP connection, hot reload                                   |
| `plugins/plugins.md`               | Condition expressions, unsafe plugins, pack support, skill lifecycle |
| `plugins/injectors.md`             | Injector reference table (timing, conditions, purpose)               |
| `plugins/plugin-tools.md`          | Plugin tool interface (Schema, Execute, Paths, Sandboxable, Init)    |
| `slash-commands/slash-commands.md` | Built-in command quick reference                                     |
| `tools/tools.md`                   | Web, media, embeddings, memory, todo, LSP tools                      |
| `ui/ui.md`                         | TUI keybindings, simple readline, web UI, headless advanced          |

## Zero Flake Policy

Every test must pass, every time. There is no concept of "skip", "xfail", or "known flaky".

- LLM-dependent tests must use `--include-tools` to constrain the model to the exact tools being tested. Do not rely on the model choosing the right tool.
- If a test depends on the LLM generating enough output (e.g., to trigger compaction), use deterministic mechanisms like `@File` directives to fill context instead.
- If read-before-write enforcement is not the subject of the test, ensure the target file does not exist (new files bypass read-before-write).
- Never add skip logic, conditional passes, or fallback assertions to suppress failures.

If a flake is encountered and cannot be fixed immediately, log it in `tests/flaky.md` with: suite name, assertion name, failure message, root cause analysis, and proposed fix.

## Key Patterns

- `--dry=render` — render config/prompts, no LLM call. Template vars (`.Agent`, `.Mode`, `.Provider`) are expanded; `.Model.Name` is empty (pre-resolution).
- `--dry=noop` — full pipeline with noop provider, no real LLM
- `--debug` → `.aura/debug.log` — tagged entries: `[tool]`, `[chat]`, `[compact]`, `[thinking]`, `[guardrail]`, `[loop]`, `[estimate]`
- `--output <file>` — mirror UI output to file
- `--include-tools` — REPLACES tool set with only specified tools (not additive)
- `--exclude-tools` — removes specified tools from the default set
- `aura tools` — lists globally available tools; does NOT apply agent/mode/task filtering
- `aura run "p1" "p2"` — non-interactive, processes prompts and exits. Never pipe or redirect stdin.

## Template Variables

Available in system prompts via Go template syntax:

| Variable               | Type              | Description                      |
| ---------------------- | ----------------- | -------------------------------- |
| `.Agent`               | string            | Agent name                       |
| `.Mode.Name`           | string            | Mode name                        |
| `.Provider`            | string            | Provider name                    |
| `.Model.Name`          | string            | Model name (empty in dry-render) |
| `.Model.Thinking`      | bool              | Model supports thinking          |
| `.Model.Vision`        | bool              | Model supports vision            |
| `.Model.ContextLength` | int               | Context window size              |
| `.Tools.Eager`         | []string          | Available eager tool names       |
| `.Vars`                | map[string]string | User-defined `--set` variables   |
| `.Sandbox.Enabled`     | bool              | Sandbox active                   |
| `.Sandbox.ReadOnly`    | []string          | Read-only paths                  |
| `.Sandbox.ReadWrite`   | []string          | Read-write paths                 |

Access `--set` vars with `{{ index .Vars "key" }}` — `.Vars.key` will error with `missingkey=error` if the key is absent.
