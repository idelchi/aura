package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/idelchi/aura/pkg/wildcard"
)

// PolicyAction is the result of evaluating a tool call against a policy.
type PolicyAction int

const (
	// PolicyAuto means the tool call runs without user confirmation.
	PolicyAuto PolicyAction = iota
	// PolicyConfirm means the tool call requires user approval before execution.
	PolicyConfirm
	// PolicyDeny means the tool call is blocked.
	PolicyDeny
)

// ToolPolicy controls which tool calls are allowed, require confirmation, or are blocked.
// Three tiers: auto (run freely), confirm (ask user), deny (hard block).
// Precedence: deny > confirm > auto > default (auto).
//
// Pattern format:
//   - "Read"              — matches the Read tool (name-only)
//   - "mcp__*"            — matches all MCP tools (glob on name)
//   - "Bash:git commit*"  — matches Bash with command matching "git commit*"
//   - "Write:/tmp/*"      — matches Write with path under /tmp/
//   - "Read:/home/*"      — matches Read with path under /home/
//
// Argument matching (the part after ":") uses the tool's primary detail:
// Bash → command, file-operating tools → path/file_path argument.
type ToolPolicy struct {
	Auto    []string `yaml:"auto"`
	Confirm []string `yaml:"confirm"`
	Deny    []string `yaml:"deny"`

	// Provenance fields for display — set by EffectiveToolPolicy, not persisted.
	approvalsProject []string
	approvalsGlobal  []string
}

// IsEmpty reports whether the policy has no rules configured.
func (p *ToolPolicy) IsEmpty() bool {
	return len(p.Auto) == 0 && len(p.Confirm) == 0 && len(p.Deny) == 0
}

// Evaluate returns the action for a given tool call.
// toolName is the tool's registered name (e.g. "Bash", "Write", "mcp__server__tool").
// args are the tool call arguments (used for argument pattern matching).
func (p *ToolPolicy) Evaluate(toolName string, args map[string]any) PolicyAction {
	if p.IsEmpty() {
		return PolicyAuto
	}

	// Deny always wins.
	if p.matchesAny(p.Deny, toolName, args) {
		return PolicyDeny
	}

	// Confirm takes precedence over auto.
	if p.matchesAny(p.Confirm, toolName, args) {
		// Check auto list (includes persisted approval rules).
		if p.matchesAny(p.Auto, toolName, args) {
			return PolicyAuto
		}

		return PolicyConfirm
	}

	// Default: auto.
	return PolicyAuto
}

// ApprovalsProject returns the project-scoped approval patterns (from config/rules/).
func (p *ToolPolicy) ApprovalsProject() []string { return p.approvalsProject }

// ApprovalsGlobal returns the global-scoped approval patterns (from ~/.aura/config/rules/).
func (p *ToolPolicy) ApprovalsGlobal() []string { return p.approvalsGlobal }

// Merge combines two policies additively.
func (p *ToolPolicy) Merge(other ToolPolicy) ToolPolicy {
	merged := ToolPolicy{
		Auto:    append(slices.Clone(p.Auto), other.Auto...),
		Confirm: append(slices.Clone(p.Confirm), other.Confirm...),
		Deny:    append(slices.Clone(p.Deny), other.Deny...),
	}

	slices.Sort(merged.Auto)
	slices.Sort(merged.Confirm)
	slices.Sort(merged.Deny)

	merged.Auto = slices.Compact(merged.Auto)
	merged.Confirm = slices.Compact(merged.Confirm)
	merged.Deny = slices.Compact(merged.Deny)

	return merged
}

// Display returns a human-facing description for system prompt injection.
func (p *ToolPolicy) Display() string {
	var parts []string

	if len(p.Auto) > 0 {
		parts = append(parts, "Auto-approved: "+strings.Join(p.Auto, ", "))
	}

	if len(p.Confirm) > 0 {
		parts = append(parts, "Requires confirmation: "+strings.Join(p.Confirm, ", "))
		parts = append(
			parts,
			"These tool calls will pause for user confirmation before executing. The user may deny them.",
		)
	}

	if len(p.Deny) > 0 {
		parts = append(parts, "Blocked: "+strings.Join(p.Deny, ", "))
	}

	parts = append(parts, "Precedence: blocked > confirmation required > auto-approved.")

	return strings.Join(parts, "\n")
}

// PolicyDisplay returns a rich human-facing policy summary for the /policy command.
func (p *ToolPolicy) PolicyDisplay(agent, mode string) string {
	if p.IsEmpty() {
		return "No tool policy configured (all tool calls auto-approved)."
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "Tool Policy (agent: %s, mode: %s):\n", agent, mode)

	writeSection := func(title string, items []string) {
		if len(items) == 0 {
			return
		}

		fmt.Fprintf(&sb, "\n  %s:\n", title)

		for _, item := range items {
			fmt.Fprintf(&sb, "    - %s\n", item)
		}
	}

	writeSection("auto", p.Auto)
	writeSection("confirm", p.Confirm)
	writeSection("deny", p.Deny)
	writeSection("approval rules (project)", p.approvalsProject)
	writeSection("approval rules (global)", p.approvalsGlobal)

	sb.WriteString("\n  Precedence: deny > confirm > auto > default (auto)")

	return sb.String()
}

// DenyError returns a formatted denial error for the given pattern.
func DenyError(pattern string) error {
	return fmt.Errorf("tool call denied by policy (pattern %q)", pattern)
}

// DenyingPattern returns the first deny pattern matching the tool call, or "".
func (p *ToolPolicy) DenyingPattern(toolName string, args map[string]any) string {
	for _, pattern := range p.Deny {
		if p.matchPattern(pattern, toolName, args) {
			return pattern
		}
	}

	return ""
}

// matchesAny reports whether any pattern in the list matches the tool call.
func (p *ToolPolicy) matchesAny(patterns []string, toolName string, args map[string]any) bool {
	for _, pattern := range patterns {
		if p.matchPattern(pattern, toolName, args) {
			return true
		}
	}

	return false
}

// matchPattern checks a single pattern against a tool call.
// Patterns without ":" match tool name only.
// Patterns with ":" match tool name + argument detail (command for Bash, path for file tools).
func (p *ToolPolicy) matchPattern(pattern, toolName string, args map[string]any) bool {
	name, arg, hasArg := strings.Cut(pattern, ":")

	if !wildcard.Match(toolName, name) {
		return false
	}

	if !hasArg {
		return true
	}

	detail := ExtractToolDetail(toolName, args)
	if detail == "" {
		return false
	}

	return wildcard.MatchMultiline(detail, arg)
}

// ExtractToolDetail returns the primary argument detail for a tool call.
// For Bash: the command string. For file-operating tools: the path/file_path argument.
// Returns "" if no detail is available.
func ExtractToolDetail(toolName string, args map[string]any) string {
	if toolName == "Bash" {
		if cmd, ok := args["command"].(string); ok {
			return cmd
		}

		return ""
	}

	for _, key := range []string{"path", "file_path"} {
		if v, ok := args[key].(string); ok && v != "" {
			return v
		}
	}

	return ""
}
