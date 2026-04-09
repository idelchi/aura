package tool

import (
	"fmt"
	"strings"
)

// Filter defines enabled/disabled glob patterns for tool filtering.
// Used by agents, modes, and tasks to control tool availability.
// Patterns support '*' wildcards (e.g. "mcp__*", "Todo*").
type Filter struct {
	// Enabled lists tool name patterns to include. Empty = all tools.
	Enabled []string
	// Disabled lists tool name patterns to exclude. Takes precedence over Enabled.
	Disabled []string
}

// IsSet returns true when at least one filter pattern is configured.
func (f Filter) IsSet() bool {
	return len(f.Enabled) > 0 || len(f.Disabled) > 0
}

// String returns a human-readable summary like "+[Query,Patch] -[Bash]".
func (f Filter) String() string {
	var parts []string

	if len(f.Enabled) > 0 {
		parts = append(parts, fmt.Sprintf("+[%s]", strings.Join(f.Enabled, ",")))
	}

	if len(f.Disabled) > 0 {
		parts = append(parts, fmt.Sprintf("-[%s]", strings.Join(f.Disabled, ",")))
	}

	return strings.Join(parts, " ")
}
