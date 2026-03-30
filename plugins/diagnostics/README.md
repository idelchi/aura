# Diagnostics

Diagnostics are development-time plugins that log, track, and surface internal state for debugging. They hook into tool calls and conversation turns to provide visibility into what the agent is doing.

Each diagnostic lives in its own subdirectory with a `plugin.yaml` and one or more `.go` files.

## Included diagnostics

| Plugin       | What it does                                                                  |
| ------------ | ----------------------------------------------------------------------------- |
| tool-logger  | Logs all tool calls and errors with result previews and error counts          |
| turn-tracker | Tracks conversation turns and context usage, warns on oversized LLM responses |
