package llamacpp

import (
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

// WithCapabilities preserves partial metadata. Preserving prior reasoning is
// independent of whether a template supports producing new reasoning.
func WithCapabilities(m model.Model, info ShowResponse) model.Model {
	if supported := info.ChatTemplateCaps.SupportsToolCalls; supported != nil {
		m.SetCapability(capabilities.Tools, *supported)
	}
	if supported := info.Modalities.Vision; supported != nil {
		m.SetCapability(capabilities.Vision, *supported)
	}
	if supported := info.ChatTemplateCaps.SupportsReasoningEffort; supported != nil {
		m.SetCapability(capabilities.ThinkingLevels, *supported)
		if *supported {
			m.SetCapability(capabilities.Thinking, true)
		}
	}
	return m
}
