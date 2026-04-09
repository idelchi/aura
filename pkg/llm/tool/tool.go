package tool

import (
	"context"
	"errors"
	"fmt"

	"github.com/idelchi/aura/pkg/wildcard"
)

// Tool represents a callable tool for LLM function calling.
type Tool interface {
	Name() string
	Description() string
	Usage() string
	Examples() string
	Schema() Schema
	Execute(ctx context.Context, args map[string]any) (string, error)
	Available() bool
}

// PreHook is optionally implemented by tools that need validation before execution
// (e.g. filetime read-before-write enforcement).
type PreHook interface {
	Pre(ctx context.Context, args map[string]any) error
}

// PostHook is optionally implemented by tools that need state updates after execution
// (e.g. filetime recording).
type PostHook interface {
	Post(ctx context.Context, args map[string]any)
}

// PathDeclarer is optionally implemented by tools that declare filesystem paths
// for sandbox pre-filtering.
type PathDeclarer interface {
	Paths(ctx context.Context, args map[string]any) (read, write []string, err error)
}

// SandboxOverride is optionally implemented by tools that override the default
// sandboxable=true behavior.
type SandboxOverride interface {
	Sandboxable() bool
}

// ParallelOverride is optionally implemented by tools that override the default
// parallel=true behavior.
type ParallelOverride interface {
	Parallel() bool
}

// LSPAware is optionally implemented by tools whose output benefits from
// LSP diagnostics on modified files.
type LSPAware interface {
	WantsLSP() bool
}

// Overrider is optionally implemented by plugin tools that replace built-in tools.
type Overrider interface {
	Overrides() bool
}

// Closer is optionally implemented by tools needing cleanup on session end.
type Closer interface {
	Close()
}

// Text holds the user-facing text for a tool (description, usage instructions, examples).
// Loaded from Go defaults and optionally overridden by YAML config.
type Text struct {
	Description string `yaml:"description"`
	Usage       string `yaml:"usage"`
	Examples    string `yaml:"examples"`
}

// Previewer is optionally implemented by tools that can generate a diff preview
// for confirmation dialogs.
type Previewer interface {
	Preview(ctx context.Context, args map[string]any) (string, error)
}

// Base provides default accessors for Description(), Usage(), Examples(), and Available() (true).
// Embed in tool structs to avoid boilerplate for the core text methods.
// Optional behaviors (Pre/Post hooks, path declaration, sandbox/parallel overrides, LSP, etc.)
// are expressed as separate opt-in interfaces — implement only the ones your tool needs.
type Base struct {
	Text Text
}

func (b Base) Description() string { return b.Text.Description }
func (b Base) Usage() string       { return b.Text.Usage }
func (b Base) Examples() string    { return b.Text.Examples }

// MergeText overwrites non-empty fields from t into the base text.
func (b *Base) MergeText(t Text) {
	if t.Description != "" {
		b.Text.Description = t.Description
	}

	if t.Usage != "" {
		b.Text.Usage = t.Usage
	}

	if t.Examples != "" {
		b.Text.Examples = t.Examples
	}
}

func (Base) Available() bool { return true }

// Tools is a collection of tools.
type Tools []Tool

var ErrToolNotFound = errors.New("tool not found")

// ErrToolCallParse indicates a provider returned tool calls with unparseable JSON arguments.
// The assistant loop can detect this with errors.Is() to retry instead of terminating.
var ErrToolCallParse = errors.New("tool call parse error")

// Get retrieves a tool by name.
func (ts Tools) Get(name string) (Tool, error) {
	for _, t := range ts {
		if t.Name() == name {
			return t, nil
		}
	}

	return nil, fmt.Errorf("%w: %q", ErrToolNotFound, name)
}

// Names returns the names of all tools.
func (ts Tools) Names() []string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.Name()
	}

	return names
}

// Schemas returns the schemas for all tools.
func (ts Tools) Schemas() Schemas {
	schemas := make([]Schema, len(ts))
	for i, t := range ts {
		schemas[i] = t.Schema()
	}

	return schemas
}

// Add adds a tool if it doesn't already exist.
func (ts *Tools) Add(t Tool) {
	if _, err := ts.Get(t.Name()); err != nil {
		*ts = append(*ts, t)
	}
}

// Remove removes a tool by name.
func (ts *Tools) Remove(name string) {
	for i, t := range *ts {
		if t.Name() == name {
			*ts = append((*ts)[:i], (*ts)[i+1:]...)

			return
		}
	}
}

// Has checks if a tool exists by name.
func (ts Tools) Has(name string) bool {
	_, err := ts.Get(name)

	return err == nil
}

// Filtered returns the list of tools that match include/exclude slices.
// Patterns support '*' wildcards (e.g. "mcp__echo__*", "Todo*").
func (ts Tools) Filtered(include, exclude []string) Tools {
	filtered := Tools{}
	includeAll := len(include) == 0

	for _, tool := range ts {
		name := tool.Name()

		if wildcard.MatchAny(name, exclude...) {
			continue
		}

		if includeAll || wildcard.MatchAny(name, include...) {
			filtered = append(filtered, tool)
		}
	}

	return filtered
}
