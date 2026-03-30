# Tools

Tools expose callable functions to the LLM, letting it take actions beyond generating text. They run inside the sandbox with full access to the conversation context.

Each tool lives in its own subdirectory with a `plugin.yaml` and one or more `.go` files.

## Included tools

| Plugin  | What it does                                                               |
| ------- | -------------------------------------------------------------------------- |
| gotify  | Sends push notifications via Gotify with configurable severity levels      |
| notepad | Reads and writes scratch files for persistent note-taking during a session |
