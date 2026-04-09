package ollama

import (
	"slices"

	"github.com/ollama/ollama/api"
	ollama "github.com/ollama/ollama/types/model"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

// WithCapabilities adds capabilities to a model based on Ollama API response.
func WithCapabilities(m model.Model, info *api.ShowResponse) model.Model {
	if slices.Contains(info.Capabilities, ollama.CapabilityTools) {
		m.Capabilities.Add(capabilities.Tools)
	}

	if slices.Contains(info.Capabilities, ollama.CapabilityEmbedding) {
		m.Capabilities.Add(capabilities.Embedding)
	}

	if slices.Contains(info.Capabilities, ollama.CapabilityThinking) {
		m.Capabilities.Add(capabilities.Thinking)
	}

	if slices.Contains(info.Capabilities, ollama.CapabilityVision) {
		m.Capabilities.Add(capabilities.Vision)
	}

	// Ollama always supports num_ctx for context override.
	m.Capabilities.Add(capabilities.ContextOverride)

	return m
}

type ModelInfo map[string]any

func contextLength(info ModelInfo) model.ContextLength {
	name, ok := info["general.architecture"].(string)
	if !ok {
		return 0
	}

	if val, ok := info[name+".context_length"]; ok {
		if length, ok := val.(float64); ok {
			return model.ContextLength(length)
		}
	}

	return 0
}
