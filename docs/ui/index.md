---
layout: default
title: UI
nav_order: 7
---

# UI

Aura supports four UI backends, all consuming the same event protocol from the backend.

| UI           | Flag          | Input                    | Features                                                              |
| ------------ | ------------- | ------------------------ | --------------------------------------------------------------------- |
| **TUI**      | default       | Interactive (Bubble Tea) | Full keybindings, text selection, status bar, spinner, picker overlay |
| **Simple**   | `--simple`    | Interactive (readline)   | History file, basic streaming                                         |
| **Headless** | _(automatic)_ | None (stdout only)       | Pure output, used for `aura run`, `aura tools`                        |
| **Web**      | `aura web`    | Browser (SSE + htmx)     | Browser-based chat, session persistence, Markdown rendering           |

## Startup Spinner

Before any UI backend starts, Aura shows a stderr spinner with progress messages covering the initialization phase (config loading, provider registry refresh, plugin compilation, MCP connections, LSP spawning). The spinner writes to stderr so it never pollutes stdout — important for `aura run` where stdout carries assistant output.

The spinner stops automatically before the UI takes the terminal.

## TUI Layout

```
┌─────────────────────────────────────────────┐
│  Chat History (scrollable viewport)         │
│                                             │
│  You: ...                                   │
│  Aura: ...                                  │
│                                             │
├─────────────────────────────────────────────┤
│  ╭──────────────────────────────────────╮   │
│  │ Input area (1-3 lines, auto-grows)   │   │
│  ╰──────────────────────────────────────╯   │
│  Agent • Mode • Model • think: level •       │
│  Provider • step X/Y • tokens: 12k/131k •   │
│  🔒 • verbose • auto                        │
│  Ctrl+T: thinking • Ctrl+R: think • ...     │
└─────────────────────────────────────────────┘
```

- **Top**: Scrollable chat history viewport
- **Middle**: Input textarea with rounded blue border (1-3 lines, grows with content)
- **Status line**: Runtime state joined with " • "
- **Help line**: Keybinding hints

## Visual Styles

| Element             | Color             | Style                                                                                 |
| ------------------- | ----------------- | ------------------------------------------------------------------------------------- |
| User messages       | Blue (#39)        | Bold                                                                                  |
| Assistant messages  | Orange (#214)     | Bold                                                                                  |
| Thinking content    | Gray (#241)       | Faint                                                                                 |
| Content             | Light gray (#252) | Normal                                                                                |
| Tool calls          | Blue (#39)        | Faint                                                                                 |
| Tool pending        | Gray (#240)       | Faint                                                                                 |
| Tool results        | Dim gray (#242)   | Faint; Read uses Chroma syntax highlighting, Rg highlights pattern matches in magenta |
| Tool errors         | Red (#196)        | Faint                                                                                 |
| Errors              | Red (#196)        | Bold                                                                                  |
| Synthetic/System    | Blue (#39)        | Faint                                                                                 |
| DisplayOnly notices | Blue (#39)        | Faint                                                                                 |
| Bookmark dividers   | Blue (#39)        | Faint, rendered as `--- label ---`                                                    |
| Input border        | Blue (#62)        | Rounded                                                                               |
| Status line         | Dim gray (#245)   | Normal                                                                                |

## Chat Rendering

- **User messages**: `"You: "` prefix + content
- **Assistant messages**: `"Aura: "` prefix, then parts in order (thinking → content → tool calls)
- **Tool calls**: 4-state lifecycle — `○` Pending (dimmed), spinner Running, `✓` Complete (with result), `✗` Error. All tool calls from a single LLM response are registered upfront as Pending before sequential execution begins
- **DisplayOnly**: Rendered as notice text (never sent to the LLM)
- **Bookmark**: Rendered as `--- label ---` divider line
- Text wrapping: ANSI-aware word wrapping to terminal width

## Keybindings

Full keybinding reference for the TUI (Bubble Tea) interface.

### Input

| Key           | Action                                                                                        |
| ------------- | --------------------------------------------------------------------------------------------- |
| `Enter`       | Send message                                                                                  |
| `Alt+Enter`   | Insert newline                                                                                |
| `Up`          | Previous input from history (when cursor at line 0)                                           |
| `Down`        | Next input from history (when cursor at last line)                                            |
| `Tab`         | Accept ghost text (directive completion or history suggestion), or cycle to next mode         |
| `Right Arrow` | Accept ghost text (directive completion or history suggestion) when cursor is at end of input |

Ghost text shows autocomplete suggestions for directives (@File, @Image, etc.) as dimmed text that can be accepted with Tab.

### Navigation

| Key             | Action                                  |
| --------------- | --------------------------------------- |
| `PgUp` / `PgDn` | Scroll viewport up/down by page         |
| `Mouse wheel`   | Scroll viewport                         |
| Auto-scroll     | Re-enabled when scrolled back to bottom |

### Control

| Key         | Action                                                                  |
| ----------- | ----------------------------------------------------------------------- |
| `Ctrl+C`    | Copy selection → clear input → cancel streaming → quit (priority order) |
| `Esc`       | Cancel current streaming                                                |
| `Shift+Tab` | Cycle to next agent                                                     |
| `Ctrl+T`    | Toggle thinking visibility in UI                                        |
| `Ctrl+R`    | Toggle thinking off ↔ on (true)                                         |
| `Ctrl+E`    | Cycle think levels (off → true → low → medium → high → off)             |
| `Ctrl+A`    | Toggle auto mode                                                        |
| `Ctrl+S`    | Toggle sandbox                                                          |
| `Ctrl+O`    | Open full output of last completed tool call in pager                   |

### Text Selection

| Action                  | Effect                     |
| ----------------------- | -------------------------- |
| Click + drag            | Select text in viewport    |
| `Ctrl+C` with selection | Copy to clipboard (OSC 52) |

### Tool Output

Tool results are syntax-highlighted in the TUI:

- **Read** — file content is highlighted using Chroma with language detection based on file extension (Monokai theme)
- **Rg** — regex pattern matches are highlighted in magenta within search results

Other tools display plain text results.

`Ctrl+O` opens the full output of the last completed tool call in a scrollable pager overlay. Tool result previews in the chat are truncated; the pager shows the complete output with syntax highlighting (for Read/Rg).

| Key                  | Action               |
| -------------------- | -------------------- |
| `Up` / `Down`        | Scroll one line      |
| `PgUp` / `PgDn`      | Scroll one page      |
| `Home` / `End`       | Jump to top / bottom |
| `Esc`, `q`, `Ctrl+C` | Close pager          |

The footer shows scroll position as a percentage when content exceeds the viewport.

### Exit Behavior

- First `Ctrl+C` (no selection, empty input, not streaming): shows "Press Ctrl+C again to quit"
- Second `Ctrl+C` within 2 seconds: exits

### Input History

File-backed history at `~/.aura/history` (TUI mode only):

| Setting            | Value                                                                  |
| ------------------ | ---------------------------------------------------------------------- |
| Max entries        | 1000                                                                   |
| File format        | One entry per line; literal `\n` for embedded newlines                 |
| Navigation         | Up/Down arrows when cursor at top/bottom                               |
| Draft preservation | Current input saved when browsing, restored when returning past newest |
| Persistence        | Best-effort write on each addition                                     |

## Status Bar

The status bar shows current runtime state at the bottom of the TUI, updated via `StatusChanged` and `DisplayHintsChanged` events. System state (agent, mode, model, tokens, steps) is carried by `StatusChanged`, while UI display preferences (verbose, auto) are carried by `DisplayHintsChanged`.

### Format

```
Agent • Mode • Model • think: level • Provider • step X/Y • tokens: 12.4k/131k (10%) • 🔒 • 📸 • verbose • auto
```

Components are joined with `•`. Empty components are omitted.

### Components

| Component    | Shown When       | Format                | Example                           |
| ------------ | ---------------- | --------------------- | --------------------------------- |
| Agent        | Always           | Agent name            | `high`                            |
| Mode         | Non-empty        | Mode name             | `Edit`                            |
| Model        | Always           | Model identifier      | `gpt-oss:20b`                     |
| Think        | Always           | `think: level`        | `think: high`                     |
| Provider     | Always           | Provider name         | `ollama`                          |
| Step counter | Iteration > 0    | `step X/Y`            | `step 3/300`                      |
| Tokens       | TokensMax > 0    | Humanized SI notation | `tokens: 12.4k/131k (10%)`        |
| Sandbox      | Always           | Lock emoji            | 🔒 when enabled, 🔓 when disabled |
| Snapshots    | SnapshotsEnabled | Camera emoji          | 📸 (git repo)                     |
| Verbose      | Verbose = true   | Text indicator        | `verbose`                         |
| Auto         | Auto = true      | Text indicator        | `auto`                            |
