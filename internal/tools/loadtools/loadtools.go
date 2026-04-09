// Package loadtools provides a meta-tool for on-demand loading of deferred tool schemas.
package loadtools

import (
	"context"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/wildcard"
)

// Inputs defines the parameters for the LoadTools tool.
type Inputs struct {
	Tools []string `json:"tools" jsonschema:"required,description=Tool names or glob patterns to load"`
}

// Tool loads deferred tool schemas on demand. When called, it marks the
// requested tools as loaded and triggers a state rebuild so they appear
// in subsequent request.Tools.
type Tool struct {
	tool.Base

	deferred tool.Tools
	onLoad   func(names []string) error
}

// New creates a LoadTools tool.
// deferred is the current set of deferred tools (for name resolution and descriptions).
// onLoad is called with the resolved tool names — it must update the loaded set and rebuild state.
func New(deferred tool.Tools, onLoad func([]string) error) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Load deferred tools by name or server so they become available for use.
					Call this when you need a tool listed as available but not yet loaded.
				`),
				Usage: heredoc.Doc(`
					After loading, tool schemas are included in subsequent requests and you
					can call them normally. Supports glob patterns with *.
				`),
				Examples: heredoc.Doc(`
					{"tools": ["Vision"]}
					{"tools": ["mcp__portainer__*"]}
					{"tools": ["Vision", "Query"]}
				`),
			},
		},
		deferred: deferred,
		onLoad:   onLoad,
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "LoadTools"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute resolves the requested tools, loads them, and returns their descriptions.
func (t *Tool) Execute(_ context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	resolved := t.resolve(params)

	if len(resolved) == 0 {
		available := t.deferred.Names()

		return "No matching deferred tools found. Available: " + strings.Join(available, ", "), nil
	}

	if err := t.onLoad(resolved); err != nil {
		return "", fmt.Errorf("loading tools: %w", err)
	}

	// Return descriptions so the model knows what was loaded.
	var parts []string

	for _, name := range resolved {
		if d, lookupErr := t.deferred.Get(name); lookupErr == nil {
			parts = append(parts, fmt.Sprintf("- %s: %s", name, firstLine(d.Description())))
		}
	}

	return fmt.Sprintf(
		"Loaded %d tools:\n%s\n\nThese tools are now available for use.",
		len(resolved),
		strings.Join(parts, "\n"),
	), nil
}

// Sandboxable returns false as this tool has no filesystem operations.
func (t *Tool) Sandboxable() bool {
	return false
}

// Parallel returns false because LoadTools triggers a full state rebuild via onLoad.
func (t *Tool) Parallel() bool {
	return false
}

// resolve matches deferred tools against the input glob patterns.
func (t *Tool) resolve(params *Inputs) []string {
	var resolved []string

	for _, d := range t.deferred {
		if wildcard.MatchAny(d.Name(), params.Tools...) {
			resolved = append(resolved, d.Name())
		}
	}

	return resolved
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}

	return s
}
