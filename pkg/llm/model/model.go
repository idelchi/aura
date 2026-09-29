package model

import (
	"slices"

	"github.com/dustin/go-humanize"

	"github.com/idelchi/aura/pkg/llm/thinking"
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
	// CapabilitiesKnown distinguishes reported capabilities from missing metadata.
	// When false, an absent capability does not establish that it is unsupported.
	CapabilitiesKnown bool `json:"capabilities_known,omitempty"`
	// KnownCapabilities tracks individual reported fields when metadata is partial.
	KnownCapabilities capabilities.Capabilities `json:"known_capabilities,omitempty"`
	// ReasoningEfforts lists explicit reasoning efforts reported for this model.
	ReasoningEfforts []thinking.Effort `json:"reasoning_efforts,omitempty"`
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

// Knows reports whether the presence or absence of a capability is authoritative.
func (m Model) Knows(capability capabilities.Capability) bool {
	return m.CapabilitiesKnown || m.KnownCapabilities.Has(capability) || m.Capabilities.Has(capability)
}

// FillMissing supplements incomplete metadata without overriding fresh reports.
func (m Model) FillMissing(previous Model) Model {
	if m.Name != previous.Name {
		return m
	}
	m.Capabilities = slices.Clone(m.Capabilities)
	m.KnownCapabilities = slices.Clone(m.KnownCapabilities)
	for _, name := range capabilities.Names() {
		capability, _ := capabilities.Parse(name)
		if !m.Knows(capability) && previous.Knows(capability) {
			m.SetCapability(capability, previous.Capabilities.Has(capability))
		}
	}
	if m.ContextLength == 0 {
		m.ContextLength = previous.ContextLength
	}
	if m.ParameterCount == 0 {
		m.ParameterCount = previous.ParameterCount
	}
	if m.Size == 0 {
		m.Size = previous.Size
	}
	if m.Family == "" {
		m.Family = previous.Family
	}
	if m.ReasoningEfforts == nil && m.Capabilities.ThinkingLevels() {
		m.ReasoningEfforts = slices.Clone(previous.ReasoningEfforts)
	}
	return m
}

// SetCapability records a reported capability, including an explicit false value.
func (m *Model) SetCapability(capability capabilities.Capability, supported bool) {
	m.KnownCapabilities.Add(capability)
	if supported {
		m.Capabilities.Add(capability)
	} else {
		m.Capabilities = slices.DeleteFunc(m.Capabilities, func(c capabilities.Capability) bool { return c == capability })
	}
}
