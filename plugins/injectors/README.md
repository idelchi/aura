# Injectors

Injectors hook into the conversation loop and inject system messages to steer LLM behavior. They fire at specific points: before a chat turn, after a response, after a tool call, or on error.

Each injector lives in its own subdirectory with a `plugin.yaml` and one or more `.go` files.

## Included injectors

| Plugin                  | What it does                                                                      |
| ----------------------- | --------------------------------------------------------------------------------- |
| done-reminder           | Reminds the LLM to call the Done tool when it stops without finishing             |
| empty-response          | Nudges the LLM when it returns an empty response                                  |
| failure-circuit-breaker | Stops after 3+ consecutive different tool failures                               |
| loop-detection          | Detects consecutive identical tool calls                                          |
| max-steps               | Disables tools and forces a summary when the iteration limit is reached           |
| repeated-patch          | Warns when a file has been patched too many times in a row                        |
| session-stats           | Injects a session summary every 5 turns (elapsed time, tool calls, context usage) |
| todo-not-finished       | Nudges the LLM when it stops but has incomplete tasks                             |
| todo-reminder           | Periodic reminder to review and update the TodoList                               |
