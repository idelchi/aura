package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/config/inherit"
	"github.com/idelchi/aura/pkg/wildcard"
	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/files"
)

// Hook defines a user-configurable shell hook that runs before or after a tool call.
// Loaded from .aura/config/hooks/**/*.yaml. The map key is the hook name.
type Hook struct {
	// Description is a human-readable summary of what this hook does.
	// Exposed to prompts via {{ .Hooks.Active }} template data.
	Description string `yaml:"description"`
	// Event is the hook timing: "pre" (before Execute) or "post" (after Execute).
	Event string `validate:"required,oneof=pre post" yaml:"event"`
	// Matcher is a regex matched against the tool name. Empty = match all tools.
	Matcher string `yaml:"matcher"`
	// Files is a glob pattern matched against file basenames (e.g. "*.go", "*.py").
	// When set, the hook only runs if at least one file path in the tool input matches.
	Files string `yaml:"files"`
	// Command is the shell command executed via sh -c. Receives JSON context on stdin.
	// $FILE env var is set to matched file paths (space-separated).
	Command string `validate:"required" yaml:"command"`
	// Timeout is the max seconds before the hook process is killed. nil = default (10s).
	Timeout *int `yaml:"timeout"`
	// Depends lists hook names that must run before this hook (within the same event).
	Depends []string `yaml:"depends"`
	// Silent suppresses all output and exit codes. The hook runs but never produces messages or blocks.
	Silent *bool `yaml:"silent"`
	// Disabled skips this hook entirely. It still appears in /hooks display but never executes.
	Disabled *bool `yaml:"disabled"`
	// Inherit lists parent hook names for config inheritance.
	Inherit []string `yaml:"inherit"`
}

// IsDisabled returns true if the hook is explicitly disabled.
func (h Hook) IsDisabled() bool { return h.Disabled != nil && *h.Disabled }

// IsEnabled returns true if the hook is active (not explicitly disabled).
func (h Hook) IsEnabled() bool { return !h.IsDisabled() }

// IsSilent returns true if the hook suppresses all output.
func (h Hook) IsSilent() bool { return h.Silent != nil && *h.Silent }

// TimeoutSeconds returns the configured timeout in seconds, or 0 if not set.
func (h Hook) TimeoutSeconds() int {
	if h.Timeout == nil {
		return 0
	}

	return *h.Timeout
}

// HookFilter controls which hooks are active for an agent or mode.
type HookFilter struct {
	// Enabled is the list of hook name patterns to include.
	Enabled []string `yaml:"enabled"`
	// Disabled is the list of hook name patterns to exclude.
	Disabled []string `yaml:"disabled"`
}

// Hooks maps hook names to their definitions.
type Hooks map[string]Hook

// Filtered returns hooks matching the include/exclude patterns.
// Patterns support '*' wildcards (same as tool filtering).
// After name filtering, hooks with unsatisfied dependencies are cascade-pruned
// because dag.Build() hard-errors on undefined parents.
func (h Hooks) Filtered(include, exclude []string) Hooks {
	if len(include) == 0 && len(exclude) == 0 {
		return h
	}

	// Normalize patterns — keys are already lowercase from Load().
	for i, p := range include {
		include[i] = strings.ToLower(p)
	}

	for i, p := range exclude {
		exclude[i] = strings.ToLower(p)
	}

	includeAll := len(include) == 0

	filtered := make(Hooks)

	for name, hook := range h {
		if wildcard.MatchAny(name, exclude...) {
			continue
		}

		if includeAll || wildcard.MatchAny(name, include...) {
			filtered[name] = hook
		}
	}

	// Cascade-prune: remove hooks whose dependencies are no longer satisfiable.
	// Fixed-point loop — repeat until no more hooks are removed.
	for changed := true; changed; {
		changed = false

		for name, hook := range filtered {
			for _, dep := range hook.Depends {
				if _, ok := filtered[dep]; !ok {
					delete(filtered, name)

					changed = true

					break
				}
			}
		}
	}

	return filtered
}

// Load reads YAML files and merges all hook definitions into the map.
// Each file contains a map of hook names to their definitions.
// Multiple hooks can live in one file, or each hook in its own file —
// the loader is decoupled from filenames.
// Supports config inheritance via the `inherit` field within hook definitions.
func (h *Hooks) Load(ff files.Files) error {
	*h = make(Hooks)

	// Phase A: Decode YAML files into Hook structs.
	all := make(map[string]Hook)

	for _, f := range ff {
		content, err := f.Read()
		if err != nil {
			return fmt.Errorf("reading hook definition %s: %w", f, err)
		}

		var fileHooks map[string]Hook
		if err := yamlutil.StrictUnmarshal(content, &fileHooks); err != nil {
			return fmt.Errorf("parsing hook definition %s: %w", f, err)
		}

		for name, hook := range fileHooks {
			for i, dep := range hook.Depends {
				hook.Depends[i] = strings.ToLower(dep)
			}

			for i, inh := range hook.Inherit {
				hook.Inherit[i] = strings.ToLower(inh)
			}

			all[strings.ToLower(name)] = hook
		}
	}

	// Phase B: Resolve inheritance (struct-level merge).
	resolved, err := inherit.Resolve(all, func(hook Hook) []string {
		return hook.Inherit
	})
	if err != nil {
		return fmt.Errorf("hook inheritance: %w", err)
	}

	// Phase C: Store resolved hooks.
	maps.Copy((*h), resolved)

	return nil
}

// Display formats hooks grouped by event in execution order.
// Pre/post name lists must already be topologically sorted (from hooks.Order)
// and contain only enabled hooks. Disabled hooks are shown separately at the end.
func (h Hooks) Display(pre, post []string) string {
	var sb strings.Builder

	writeHook := func(name string) {
		hook := h[name]

		fmt.Fprintf(&sb, "\n  %s\n", name)

		if hook.Description != "" {
			fmt.Fprintf(&sb, "    description: %s\n", hook.Description)
		}

		if hook.IsDisabled() {
			fmt.Fprintf(&sb, "    disabled: true\n")
		}

		if len(hook.Inherit) > 0 {
			fmt.Fprintf(&sb, "    inherit:  %s\n", strings.Join(hook.Inherit, ", "))
		}

		if len(hook.Depends) > 0 {
			fmt.Fprintf(&sb, "    depends:  [%s]\n", strings.Join(hook.Depends, ", "))
		}

		if hook.Matcher != "" {
			fmt.Fprintf(&sb, "    matcher:  %s\n", hook.Matcher)
		}

		if hook.Files != "" {
			fmt.Fprintf(&sb, "    files:    %s\n", hook.Files)
		}

		if hook.IsSilent() {
			fmt.Fprintf(&sb, "    silent:   true\n")
		}

		if hook.Command != "" {
			fmt.Fprintf(&sb, "    command:  %s\n", formatCommand(hook.Command))
		}

		if hook.TimeoutSeconds() > 0 {
			fmt.Fprintf(&sb, "    timeout:  %s\n", (time.Duration(hook.TimeoutSeconds()) * time.Second).String())
		}
	}

	writeGroup := func(label string, names []string) {
		if len(names) == 0 {
			fmt.Fprintf(&sb, "No %s hooks configured.\n", label)

			return
		}

		fmt.Fprintf(&sb, "%s hooks (execution order):\n", strings.ToUpper(label[:1])+label[1:])

		for _, name := range names {
			writeHook(name)
		}
	}

	writeGroup("post", post)

	if len(post) > 0 && len(pre) > 0 {
		sb.WriteString("\n")
	}

	writeGroup("pre", pre)

	// Show disabled hooks separately so users know they exist.
	var disabled []string

	for name, hook := range h {
		if hook.IsDisabled() {
			disabled = append(disabled, name)
		}
	}

	slices.Sort(disabled)

	if len(disabled) > 0 {
		sb.WriteString("\n\nDisabled hooks:")

		for _, name := range disabled {
			writeHook(name)
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// formatCommand returns the first line of a command, appending a count if multiline.
func formatCommand(cmd string) string {
	lines := strings.Split(strings.TrimRight(cmd, "\n"), "\n")
	first := lines[0]

	if len(lines) > 1 {
		return fmt.Sprintf("%s (+ %d lines)", first, len(lines)-1)
	}

	return first
}
