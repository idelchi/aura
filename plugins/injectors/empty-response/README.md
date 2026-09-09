# empty-response

Injector plugin. When the LLM returns an empty response, injects a nudge telling it to continue with the task or explain what it needs.

`disable_tools: ["Read", "Glob"]` closes access to matching tools for the current turn and asks for a final answer
from existing evidence, or an explicit incomplete result. Other tools remain available. Default `[]` preserves
ordinary recovery behavior; `["*"]` disables every tool. Configure it under
`features.plugins.config.local.empty-response` for short assessment tasks, not workflows that must continue tool work.
The normal task deadline and max-step limit still bound repeated empty answers.
