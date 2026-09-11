# response-validation

Opt-in final-answer validation using the agent's existing `response_format`.
Aura checks JSON syntax and JSON Schema; this plugin decides whether to request a correction or fail the turn.
It does not verify factual correctness. Tool-call responses are not final answers.

`retries: 1` allows correction within the first two model iterations of a user turn; previous tool iterations count
toward that bound. Correction disables tools to avoid repeating side effects. An invalid answer after the bound
produces an explicit policy failure. Agents without `response_format` are unaffected.
