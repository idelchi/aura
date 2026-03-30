// Package capabilities defines model capability flags.
package capabilities

import (
	"fmt"
	"slices"
	"strings"
)

// Capability represents a model capability flag.
type Capability string

// Capabilities is a collection of model capability flags.
type Capabilities []Capability

const (
	// Thinking indicates support for reasoning/thinking output.
	Thinking Capability = "Thinking"
	// ThinkingLevels indicates support for configurable thinking depth.
	ThinkingLevels Capability = "ThinkingLevels"
	// Tools indicates support for tool/function calling.
	Tools Capability = "Tools"
	// Embedding indicates this is an embedding model.
	Embedding Capability = "Embedding"
	// Reranking indicates support for reranking operations.
	Reranking Capability = "Reranking"
	// Vision indicates support for vision input.
	Vision Capability = "Vision"
	// ContextOverride indicates the provider respects request.ContextLength.
	ContextOverride Capability = "ContextOverride"
)

// Add adds a capability if not already present.
func (cs *Capabilities) Add(cap Capability) {
	if !cs.Has(cap) {
		*cs = append(*cs, cap)
	}
}

// Has returns true if the capability is present.
func (cs Capabilities) Has(cap Capability) bool {
	return slices.Contains(cs, cap)
}

// Thinking returns true if thinking capability is present.
func (cs Capabilities) Thinking() bool {
	return cs.Has(Thinking)
}

// ThinkingLevels returns true if thinking levels are supported.
func (cs Capabilities) ThinkingLevels() bool {
	return cs.Has(ThinkingLevels)
}

// Tools returns true if tool calling is supported.
func (cs Capabilities) Tools() bool {
	return cs.Has(Tools)
}

// Reranking returns true if reranking is supported.
func (cs Capabilities) Reranking() bool {
	return cs.Has(Reranking)
}

// Embedding returns true if this is an embedding model.
func (cs Capabilities) Embedding() bool {
	return cs.Has(Embedding)
}

// Vision returns true if vision/image input is supported.
func (cs Capabilities) Vision() bool {
	return cs.Has(Vision)
}

// ContextOverride returns true if the provider respects request.ContextLength.
func (cs Capabilities) ContextOverride() bool {
	return cs.Has(ContextOverride)
}

// known maps user-facing names (snake_case) to capability constants.
// This is the single source of truth for parsing, validation, and display.
// The userFacing flag controls whether the capability appears in FilterNames().
var known = []struct {
	name       string
	cap        Capability
	userFacing bool
}{
	{"thinking", Thinking, true},
	{"thinking_levels", ThinkingLevels, false},
	{"tools", Tools, true},
	{"embedding", Embedding, true},
	{"reranking", Reranking, true},
	{"vision", Vision, true},
	{"context_override", ContextOverride, false},
}

// Parse converts a string to a Capability (case-insensitive).
// Accepts snake_case names ("thinking_levels") and PascalCase constants ("ThinkingLevels").
func Parse(s string) (Capability, error) {
	lower := strings.ToLower(s)

	for _, k := range known {
		if k.name == lower || strings.ToLower(string(k.cap)) == lower {
			return k.cap, nil
		}
	}

	return "", fmt.Errorf("unknown capability %q; valid values: %s", s, strings.Join(Names(), ", "))
}

// Names returns the snake_case names of all known capabilities.
func Names() []string {
	names := make([]string, 0, len(known))
	for _, k := range known {
		names = append(names, k.name)
	}

	return names
}

// FilterNames returns only the user-facing capability names for CLI filter help text.
// Internal capabilities (thinking_levels, context_override) are excluded.
func FilterNames() []string {
	var names []string

	for _, k := range known {
		if k.userFacing {
			names = append(names, k.name)
		}
	}

	return names
}

// Map returns a flat map of capability names to booleans.
// Keys are snake_case to match condition syntax (e.g. "model_has:vision").
func (cs Capabilities) Map() map[string]bool {
	return map[string]bool{
		"thinking":         cs.Thinking(),
		"thinking_levels":  cs.ThinkingLevels(),
		"tools":            cs.Tools(),
		"vision":           cs.Vision(),
		"embedding":        cs.Embedding(),
		"reranking":        cs.Reranking(),
		"context_override": cs.ContextOverride(),
	}
}
