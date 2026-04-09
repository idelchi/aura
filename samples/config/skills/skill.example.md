---
# EXAMPLE — This file lives under config/skills/ for reference only — it is NOT loaded from here.
# To create a skill, rename to <name>.md and place in <home>/skills/ (one level above config/).
# Full reference of all skill frontmatter fields.

# Unique identifier. The LLM invokes this skill by name.
name: refactor

# Human-readable description. Visible in the tool schema (progressive disclosure).
# The LLM sees this description and decides when to invoke the skill.
description: >
  Invoke when you need to refactor code — rename symbols, extract functions, restructure modules,
  or simplify complex logic. Use this skill instead of ad-hoc edits when the change touches
  multiple files or requires understanding caller/callee relationships.
---

# Refactoring Skill

You have been invoked to perform a structured refactoring. Follow these steps precisely:

## 1. Understand the Scope

- Read all files involved in the refactoring
- Grep for every call site, reference, and import of the symbol being changed
- Map out the dependency graph before making any changes

## 2. Plan the Change

Before editing, state:

- What is being refactored and why
- Which files will be touched
- What the before/after signatures or structure look like
- Any risks or edge cases

## 3. Execute

- Make all changes atomically — do not leave the codebase in a half-refactored state
- Update all callers, tests, and documentation in the same pass
- If a test breaks, fix the code (not the test) unless the test is testing the old behavior

## 4. Verify

- Run `go build ./...` (or equivalent) to ensure compilation
- Run `go test ./...` to ensure tests pass
- Grep once more for any remaining references to the old symbol

## 5. Report

Summarize what changed: files touched, symbols renamed, callers updated, tests affected.
