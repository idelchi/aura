# SDK Changelog

## 0.0.0

Initial semver release. Establishes the plugin contract.

### Hook Return Types

- `Result` — hook return with message injection, tool disabling, and timing-specific modifications
- `BeforeToolResult` — extends Result with argument rewriting (`Arguments`) and execution blocking (`Block`)
- `ResponseModification` — skip response or replace content (AfterResponse only)
- `ErrorModification` — retry or suppress errors (OnError only)
- `RequestModification` — append to system prompt or skip chat (BeforeChat only)
- `CompactionModification` — suppress built-in compaction (BeforeCompaction only)

### Hook Context Types

- `Context` — base context with session state, model info, tools, features, tokens, todos, turns, plugin config
- `BeforeChatContext` — embeds Context
- `AfterResponseContext` — embeds Context, adds response content, thinking, and pending tool calls
- `BeforeToolContext` — embeds Context, adds tool name and arguments
- `AfterToolContext` — embeds Context, adds tool result, error, and duration
- `OnErrorContext` — embeds Context, adds error message, classified type, retryable flag, status code
- `BeforeCompactionContext` — embeds Context, adds forced flag, token usage, message count, keep-last
- `AfterCompactionContext` — embeds Context, adds success flag, pre/post message counts, summary length
- `OnAgentSwitchContext` — embeds Context, adds previous/new agent names and switch reason
- `TransformContext` — embeds Context, adds ForLLM()-filtered messages with positional IDs

### Domain Types

- `Role` — string type for message roles
- `ToolCall` — completed tool execution (name, args, JSON args, result, error, duration)
- `Message` — conversation message for transform plugins (positional ID, role, content, thinking, tool calls, tokens, type, timestamp)
- `MessageToolCall` — tool call within a message (provider ID, name, arguments)
- `Turn` — text-only conversation turn (role, content)
- `TokenState` — token usage from multiple sources (estimate, last API, percent, max)
- `Stats` — cumulative session metrics (time, turns, iterations, tool calls/errors, tokens in/out, top tools)
- `ToolCount` — tool name paired with invocation count (used by Stats.TopTools)
- `ModelInfo` — resolved model metadata (name, family, parameter count, context length, capabilities)
- `FeatureState` — runtime feature toggles (sandbox, read-before-write, thinking display, compaction)
- `SandboxFeatureState` — sandbox toggle state (enabled vs requested)

### Tool Plugin Types

- `ToolSchema` — tool function-calling schema (name, description, usage, examples, parameters)
- `ToolParameters` — parameter object definition (type, properties, required)
- `ToolProperty` — single parameter definition (type, description, enum)
- `ToolConfig` — runtime paths passed to optional Init() (home dir, config dir)
- `ToolPaths` — filesystem path declarations (sandbox read/write, filetime record/guard/clear)

### Command Plugin Types

- `CommandSchema` — slash command metadata (name, description, hints, forward, silent)
- `CommandResult` — command execution output

### Functions

- `ValidateTransformed([]Message) error` — structural validation for TransformMessages output (non-empty, system-first, no orphaned tool results)

### Constants

- `Version = "0.0.0"` (in `sdk/version/version.go`)
- `RoleUser`, `RoleAssistant` — message role constants

### Yaegi Integration

- `Symbols` — export map for `interp.Use()` registering all SDK types and constants
