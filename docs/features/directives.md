---
layout: default
title: Directives
parent: Features
nav_order: 9
---

# Input Directives

Directives are special syntax in user input that trigger preprocessing before the message is sent to the LLM.

## @Image

Embed images directly into user messages for vision-capable models. Images are resized and JPEG-compressed before attachment. If the current model doesn't support vision, a warning is shown (message still sent).

```
@Image[screenshot.png] What's wrong with this UI?
```

Configure in `.aura/config/features/vision.yaml`:

| Setting     | Default | Description                           |
| ----------- | ------- | ------------------------------------- |
| `dimension` | 1024    | Max pixel dimension (width or height) |
| `quality`   | 75      | JPEG compression quality (1-100)      |

## @Bash

Run a shell command and inject its output as context before the user message. Output (including errors) is placed in a preamble block.

```
@Bash[git log --oneline -5] Summarize recent commits
```

## @File

Inject file contents (or a directory listing) as context before the user message. Files over 500 lines are truncated; directories produce a shallow listing.

```
@File[src/main.go] What does this file do?
```

## @Path

Replace the directive with a bare path, expanding environment variables inline — no file reading.

```
@Path[$HOME/.config/aura] Check this path
```

## Autocomplete

In the TUI, typing `@` triggers directive completion:

- `@Image[` shows a file picker filtered to image files (PNG, JPG, GIF, WebP, BMP)
- Press `Tab` or `Right Arrow` to accept the completion

## Input Size Guard

Messages using `@Bash` or `@File` directives that would push context usage above `user_input_max_percentage` (default: 80%) are rejected before entering the conversation. The error is shown in the TUI and the message is not sent.

Configure the threshold in `.aura/config/features/tools.yaml`:

```yaml
tools:
  user_input_max_percentage: 85
```

## Argument Splitting

Slash command arguments use quote-aware splitting (via [shlex](https://github.com/google/shlex)). This means you can use quoted strings with spaces as single arguments:

```
/assert "context_above:70 and auto" "/compact"
/assert bash:"go build ./..." "Fix the build errors."
```

If quote parsing fails (e.g. unmatched quotes in natural text), arguments fall back to whitespace splitting.

## Supported Formats

Images and PDFs: PNG, JPG, JPEG, GIF, WebP, BMP, PDF

The `@Image` directive handles PDFs by extracting one image per page. For single-image analysis and text extraction, the `Vision` tool is also available.
