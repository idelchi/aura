---
layout: default
title: aura skills
parent: Commands
nav_order: 13
---

# aura skills

Manage LLM-invocable skills.

## Syntax

```sh
aura skills add <source> [source...] [flags]
aura skills update [name...] [flags]
aura skills remove <name> [name...]
```

## Description

Skills are Markdown files with YAML frontmatter that provide multi-step instructions to the LLM. The LLM invokes them via the `Skill` tool during a conversation. The `aura skills` command manages installation, updates, and removal. Use [`aura show skills`]({{ site.baseurl }}/commands/show) to list and inspect skills.

See [Skills]({{ site.baseurl }}/features/tools#skills) for details on writing skills.

## Subcommands

| Subcommand                 | Description                                                            |
| -------------------------- | ---------------------------------------------------------------------- |
| `add <source> [source...]` | Install skills from git URLs, local directories, or single `.md` files |
| `update [name...]`         | Pull latest from git origin                                            |
| `remove <name> [name...]`  | Delete skill files or directories                                      |

## Add Flags

| Flag        | Default          | Description                                                        |
| ----------- | ---------------- | ------------------------------------------------------------------ |
| `--name`    | (derived)        | Custom directory name for the skill                                |
| `--ref`     | (default branch) | Git ref to checkout (tag, branch, or commit)                       |
| `--subpath` |                  | Subdirectories within the source to use as skill root (repeatable) |
| `--global`  | `false`          | Install to `~/.aura/skills/` instead of local                      |

## Update Flags

| Flag    | Default | Description                   |
| ------- | ------- | ----------------------------- |
| `--all` | `false` | Update all git-sourced skills |

## Naming

The skill name defaults to the last path segment of the source URL, stripped of the `aura-skill-` prefix:

- `github.com/user/aura-skill-commit` &rarr; `commit/`
- `github.com/user/git-skills` &rarr; `git-skills/`

When a single `--subpath` is provided without `--name`, the name is derived from the subpath's last segment:

- `--subpath skills/commit` &rarr; `commit/`

With multiple `--subpath` flags, the name falls back to URL-based derivation.

For single `.md` files, the name defaults to the filename without extension.

Override with `--name` (single source only — `--name`, `--ref`, and `--subpath` cannot be used with multiple sources).

## Skill Format

```markdown
---
name: my-skill
description: When and why the LLM should invoke this skill.
---

Instructions for the LLM to follow when this skill is invoked.
```

A skill repo can contain multiple `.md` files, each becoming a separate skill.

## Authentication

Same as `aura plugins` — see [plugin authentication]({{ site.baseurl }}/commands/plugin#authentication).

## Examples

```sh
# List all skills
aura skills list

# Inspect a skill
aura skills show commit

# Install from git
aura skills add https://github.com/user/aura-skill-commit

# Install a skill pack (multiple skills in one repo)
aura skills add https://github.com/user/git-skills

# Install a single .md file
aura skills add ./my-skill.md

# Install from local directory
aura skills add ./path/to/skills --name my-skills

# Install globally
aura skills add https://github.com/user/aura-skill-commit --global

# Update a specific skill
aura skills update commit

# Update all git-sourced skills
aura skills update --all

# Remove a skill
aura skills remove commit
```
