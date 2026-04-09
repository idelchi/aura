---
layout: default
title: Features
nav_order: 5
has_children: true
---

# Features

| Feature                                                                          | Description                                                                     |
| -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| [Tools]({{ site.baseurl }}/features/tools)                                       | Built-in tools + custom tools via Go plugins                                    |
| [Slash Commands]({{ site.baseurl }}/features/slash-commands)                     | Built-in commands + user-defined custom commands                                |
| [Skills]({{ site.baseurl }}/features/tools#skills)                               | LLM-invocable capabilities with progressive disclosure                          |
| [Sessions]({{ site.baseurl }}/features/sessions)                                 | Save, resume, and fork conversations                                            |
| [Embeddings]({{ site.baseurl }}/features/embeddings)                             | Embedding-based codebase search with AST-aware chunking                         |
| [Compaction]({{ site.baseurl }}/features/compaction)                             | Automatic context compression via dedicated agent                               |
| [Thinking]({{ site.baseurl }}/features/thinking)                                 | Extended reasoning with configurable levels                                     |
| [Sandboxing]({{ site.baseurl }}/features/sandboxing)                             | Landlock LSM filesystem restrictions with per-tool path declarations            |
| [MCP]({{ site.baseurl }}/features/mcp)                                           | Model Context Protocol support                                                  |
| [Directives]({{ site.baseurl }}/features/directives)                             | Input preprocessing directives: @Image, @Bash, @File, @Path                     |
| [Hooks]({{ site.baseurl }}/features/hooks)                                       | Shell hooks before/after tool execution, plus condition-based message injectors |
| [LSP]({{ site.baseurl }}/features/lsp)                                           | Language server diagnostics appended to tool results after execution            |
| [Guardrail]({{ site.baseurl }}/features/guardrail)                               | Secondary LLM validation of tool calls and user messages                        |
| [Plugins]({{ site.baseurl }}/features/plugins)                                   | User-defined Go plugins via Yaegi for programmatic lifecycle hooks              |
| [Memory]({{ site.baseurl }}/features/tools#memory)                               | Persistent key-value storage across sessions and compactions                    |
| [Deferred Tools]({{ site.baseurl }}/features/tools#deferred-tools)               | On-demand tool loading to reduce initial context usage                          |
| [Scheduled Tasks]({{ site.baseurl }}/commands/task)                              | Cron-based task scheduling with foreach iteration and runtime templates         |
| [Replay]({{ site.baseurl }}/features/slash-commands#assert--conditional-actions) | YAML-based command sequences with Go templating and conditional logic           |
| [Web UI]({{ site.baseurl }}/commands/web)                                        | Browser-based chat interface with SSE streaming                                 |
