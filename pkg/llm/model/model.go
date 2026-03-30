package model

import (
	"github.com/dustin/go-humanize"

	"github.com/idelchi/aura/pkg/providers/capabilities"
	"github.com/idelchi/aura/pkg/wildcard"
)

// Model represents an LLM model with metadata.
type Model struct {
	// Name is the model identifier.
	Name string
	// ParameterCount is the model's parameter count (e.g., 8B = 8_000_000_000).
	ParameterCount ParameterCount `json:"parameter_count,omitempty"`
	// ContextLength is the maximum context window size.
	ContextLength ContextLength `json:"context_length"`
	// Capabilities lists the model's supported features.
	Capabilities capabilities.Capabilities `json:",omitempty"`
	// Family is the model family name (e.g., "gpt", "llama").
	Family string `json:",omitempty"`
	// Size is the model size in bytes.
	Size uint64 `json:",omitempty"`
}

// String returns the model name.
func (m Model) String() string {
	return m.Name
}

// Deref safely dereferences a *Model pointer.
// Returns the Model value if non-nil, or a zero-value Model if nil.
// Safe to call on nil receivers.
func (m *Model) Deref() Model {
	if m == nil {
		return Model{}
	}

	return *m
}

// ContextLength represents a context window size.
type ContextLength int

// String returns a human-readable SI format (e.g., "8k", "128k").
func (c ContextLength) String() string {
	return humanize.SIWithDigits(float64(c), 0, "")
}

// PercentUsed returns the percentage of context used by the given input token count.
func (c ContextLength) PercentUsed(inputTokens int) float64 {
	if c == 0 {
		return 0
	}

	return float64(inputTokens) / float64(c) * 100
}

// Matches returns true if the pattern matches this model's name.
func (m Model) Matches(pattern string) bool {
	return wildcard.MatchAny(pattern, m.Name)
}
