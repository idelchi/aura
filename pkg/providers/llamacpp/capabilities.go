package llamacpp

import (
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

// WithCapabilities adds capabilities based on the model's chat template capabilities.
func WithCapabilities(m model.Model, info ShowResponse) model.Model {
	if info.ChatTemplateCaps.SupportsToolCalls {
		m.Capabilities.Add(capabilities.Tools)
	}

	if info.ChatTemplateCaps.SupportsPreserveReasoning {
		m.Capabilities.Add(capabilities.Thinking)
	}

	if info.Modalities.Vision {
		m.Capabilities.Add(capabilities.Vision)
	}

	return m
}
