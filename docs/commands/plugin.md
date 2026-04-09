---
layout: default
title: aura plugins
parent: Commands
nav_order: 12
---

# aura plugins

Manage Go plugins.

## Syntax

```sh
aura plugins add <source> [source...] [flags]
aura plugins update [name|pack...] [flags]
aura plugins remove <name|pack> [name|pack...]
```

## Description

Plugins are user-defined Go modules that hook into the conversation lifecycle via the [Yaegi](https://github.com/cogentcore/yaegi) interpreter. Use [`aura show plugins`]({{ site.baseurl }}/commands/show) to list and inspect installed plugins.

See [Plugins]({{ site.baseurl }}/features/plugins) for details on writing plugins.

## Flags

**add:**

| Flag          | Default          | Description                                                       |
| ------------- | ---------------- | ----------------------------------------------------------------- |
| `--name`      | (derived)        | Custom directory name for the plugin                              |
| `--ref`       | (default branch) | Git ref to checkout (tag, branch, or commit)                      |
| `--subpath`   |                  | Subdirectory within the source to use as plugin root (repeatable) |
| `--global`    | `false`          | Install to `~/.aura/plugins/` instead of local                    |
| `--no-vendor` | `false`          | Skip `go mod tidy` and `go mod vendor` after installing           |

**update:**

| Flag          | Default | Description                                           |
| ------------- | ------- | ----------------------------------------------------- |
| `--all`       | `false` | Update all git-sourced plugins                        |
| `--no-vendor` | `false` | Skip `go mod tidy` and `go mod vendor` after updating |

## Naming

The plugin name defaults to the last path segment of the source URL, stripped of the `aura-plugin-` prefix:

- `github.com/user/aura-plugin-metrics` → `metrics/`
- `github.com/user/my-hooks` → `my-hooks/`

With a single `--subpath`, the name comes from the subpath's last segment. With multiple `--subpath` flags, the URL-based name is used (the repo is cloned as a pack). Use `--name` to override (single source only).

## Plugin Packs

A single repository can contain multiple plugins in subdirectories. Each subdirectory needs its own `plugin.yaml`, `go.mod`, and `.go` files:

```
github.com/user/aura-plugins/
  gotify/
    plugin.yaml  go.mod  main.go
  slack/
    plugin.yaml  go.mod  main.go
```

All plugins are discovered and installed together. Update and remove operate at the pack level — use `disabled: true` in `plugin.yaml` to skip individual plugins within a pack.

## Authentication

**HTTPS:** No auth (public) → `GIT_USERNAME`/`GIT_PASSWORD`, `GITHUB_TOKEN`, `GITLAB_TOKEN`, `GIT_TOKEN` → `git credential fill`

**SSH:** SSH agent → key files (`~/.ssh/id_ed25519`, `id_rsa`, `id_ecdsa`)

## Examples

```sh
# Install from git
aura plugins add https://github.com/user/aura-plugin-metrics

# Install a specific tag
aura plugins add https://github.com/user/aura-plugin-metrics --ref v0.2.0

# Install globally
aura plugins add https://github.com/user/aura-plugin-metrics --global

# Install from local path
aura plugins add ./path/to/my-plugin --name my-plugin

# Install a plugin pack
aura plugins add https://github.com/user/aura-plugins

# Update all plugins
aura plugins update --all

# Update a specific plugin (resolves to its pack)
aura plugins update gotify

# Remove a plugin or pack
aura plugins remove my-standalone
aura plugins remove tools
```
