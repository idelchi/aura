package model

import (
	"cmp"
	"slices"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/pkg/providers/capabilities"
	"github.com/idelchi/aura/pkg/wildcard"
)

// Models is a collection of model metadata.
type Models []Model

// FromNames creates a Models collection from model names.
func FromNames(names []string) Models {
	var ms Models

	for _, name := range names {
		ms = append(ms, Model{Name: name})
	}

	return ms
}

// Names returns a slice of model names.
func (ms Models) Names() []string {
	var names []string

	for _, m := range ms {
		names = append(names, m.Name)
	}

	return names
}

// Exists returns true if a model with the given name exists.
func (ms Models) Exists(name string) bool {
	return slices.ContainsFunc(ms, func(m Model) bool { return m.Name == name })
}

// Get returns the model with the given name, or zero value if not found.
func (ms Models) Get(name string) Model {
	for _, m := range ms {
		if m.Name == name {
			return m
		}
	}

	return Model{}
}

// HasTools returns models with tool calling capability.
func (ms Models) HasTools() Models {
	var capable Models

	for _, m := range ms {
		if m.Capabilities.Tools() {
			capable = append(capable, m)
		}
	}

	return capable
}

// IsEmbedding returns embedding models.
func (ms Models) IsEmbedding() Models {
	var capable Models

	for _, m := range ms {
		if m.Capabilities.Embedding() {
			capable = append(capable, m)
		}
	}

	return capable
}

// IsGeneral returns general-purpose models (neither embedding nor tool-specific).
func (ms Models) IsGeneral() Models {
	var general Models

	for _, m := range ms {
		if !m.Capabilities.Embedding() && !m.Capabilities.Tools() {
			general = append(general, m)
		}
	}

	return general
}

// WithCapability returns models that have the given capability.
func (ms Models) WithCapability(cap capabilities.Capability) Models {
	var out Models

	for _, m := range ms {
		if m.Capabilities.Has(cap) {
			out = append(out, m)
		}
	}

	return out
}

// LongestName returns the display width of the longest model name.
func (ms Models) LongestName() int {
	longest := 0

	for _, m := range ms {
		if w := ansi.StringWidth(m.Name); w > longest {
			longest = w
		}
	}

	return longest
}

// ByName returns models sorted alphabetically by name.
func (ms Models) ByName() Models {
	out := slices.Clone(ms)

	slices.SortFunc(out, func(a, b Model) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return out
}

// BySize returns models sorted by size (descending), with context length as fallback.
func (ms Models) BySize() Models {
	out := slices.Clone(ms)

	slices.SortFunc(out, func(a, b Model) int {
		if a.Size != b.Size {
			return cmp.Compare(b.Size, a.Size)
		}

		// Fallback to context length
		return cmp.Compare(b.ContextLength, a.ContextLength)
	})

	return out
}

// ByContextLength returns models sorted by context length (descending), with name as fallback.
func (ms Models) ByContextLength() Models {
	out := slices.Clone(ms)

	slices.SortFunc(out, func(a, b Model) int {
		if a.ContextLength != b.ContextLength {
			return cmp.Compare(b.ContextLength, a.ContextLength)
		}

		// Fallback to name
		return cmp.Compare(a.Name, b.Name)
	})

	return out
}

// SortBy represents the field to sort models by.
type SortBy string

const (
	SortByName    SortBy = "name"
	SortByContext SortBy = "context"
	SortBySize    SortBy = "size"
)

// Sort returns models sorted by the specified field.
func (ms Models) Sort(by SortBy) Models {
	switch by {
	case SortByName:
		return ms.ByName()
	case SortByContext:
		return ms.ByContextLength()
	case SortBySize:
		return ms.BySize()
	default:
		return ms.BySize()
	}
}

// Exclude returns models that do not match any of the given wildcard patterns.
func (ms Models) Exclude(patterns ...string) Models {
	if len(patterns) == 0 {
		return ms
	}

	var out Models

	for _, m := range ms {
		if wildcard.MatchAny(m.Name, patterns...) {
			continue
		}

		out = append(out, m)
	}

	return out
}

// Include returns models matching any of the given wildcard patterns.
func (ms Models) Include(patterns ...string) Models {
	if len(patterns) == 0 {
		return ms
	}

	var out Models

	for _, m := range ms {
		if wildcard.MatchAny(m.Name, patterns...) {
			out = append(out, m)
		}
	}

	return out
}
