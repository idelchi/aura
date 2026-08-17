package ollama

import (
	"slices"
	"strings"

	"github.com/ollama/ollama/api"
	ollama "github.com/ollama/ollama/types/model"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/thinking"
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
		m.Capabilities.Add(capabilities.ThinkingLevels)

		m.ReasoningEfforts = ollamaReasoningEfforts(m.Family)
	}

	if slices.Contains(info.Capabilities, ollama.CapabilityVision) {
		m.Capabilities.Add(capabilities.Vision)
	}

	// Ollama always supports num_ctx for context override.
	m.Capabilities.Add(capabilities.ContextOverride)

	return m
}

func ollamaReasoningEfforts(family string) []thinking.Effort {
	// GPT-OSS is the documented exception to Ollama's usual maximum effort:
	// it accepts low, medium, and high, but not max.
	if normalized := strings.ToLower(family); normalized == "gptoss" || normalized == "gpt-oss" {
		return []thinking.Effort{thinking.Low, thinking.Medium, thinking.High}
	}

	return []thinking.Effort{thinking.Low, thinking.Medium, thinking.High, thinking.Max}
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
