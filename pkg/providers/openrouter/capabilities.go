package openrouter

import (
	"slices"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

// SupportedParameters contains parameter names supported by a model.
type SupportedParameters []string

// WithCapabilities adds capabilities to a model based on supported parameters and input modalities.
func WithCapabilities(m model.Model, info SupportedParameters, inputModalities []string) model.Model {
	if slices.Contains(info, "reasoning") {
		m.Capabilities.Add(capabilities.Thinking)
	}

	if slices.Contains(info, "reasoning_effort") {
		m.Capabilities.Add(capabilities.ThinkingLevels)
	}

	if slices.Contains(info, "tools") {
		m.Capabilities.Add(capabilities.Tools)
	}

	if slices.Contains(inputModalities, "image") {
		m.Capabilities.Add(capabilities.Vision)
	}

	return m
}
