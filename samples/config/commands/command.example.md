---
# EXAMPLE — This file is not loaded. Rename to <name>.md to use.
# Full reference of all custom slash command frontmatter fields.

# Unique identifier. Becomes /name in the command list.
name: explain

# Human-readable description. Shown in /help listing.
description: Explain a function, file, or concept from the codebase

# Argument hint text shown as TUI ghost text when typing the command.
hints: <function-or-file> [depth]
---

Explain $1 in detail.

Cover:

- What it does and why it exists
- How it fits into the broader architecture
- Key design decisions and trade-offs
- Any non-obvious behavior or edge cases

$ARGUMENTS

Tailor the explanation to a developer who is new to this codebase but experienced with the language.
