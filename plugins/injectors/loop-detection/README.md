# loop-detection

Injector plugin. Fires after each tool call. If the last two tool calls have the same name and identical arguments, warns the LLM that it's looping and should try a different approach.

`window` sets the number of consecutive identical calls (minimum 2).
`disable_tool: true` additionally disables that tool for the current turn after repeated **successful** calls.
Execution failures do not trigger disabling. Requests with changed arguments (such as a smaller tail after an
oversized result) do not count as identical; repeating the same oversized request can trigger disabling.
Default behavior remains advisory. Configure this under `features.plugins.config.local.loop-detection` for agents/tasks
where repeating a successful read is not useful; do not enable it for intentional polling workflows.
