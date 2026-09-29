// Package catalog reads metadata shared by OpenAI-compatible model catalogs.
package catalog

import (
	"encoding/json"
	"slices"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

// Model contains optional extensions used by local servers and gateways.
// Missing fields are unknown, not negative capability reports.
type Model struct {
	ID                  string          `json:"id"`
	ContextLength       int             `json:"context_length"`
	ContextWindow       int             `json:"context_window"`
	Capabilities        map[string]bool `json:"capabilities"`
	SupportedParameters []string        `json:"supported_parameters"`
	Architecture        struct {
		InputModalities []string `json:"input_modalities"`
	} `json:"architecture"`
	Meta struct {
		ContextLength int `json:"n_ctx"`
	} `json:"meta"`
}

// Decode enriches a model with catalog metadata, without inventing unsupported flags.
func Decode(data []byte) (model.Model, error) {
	var entry Model
	if err := json.Unmarshal(data, &entry); err != nil {
		return model.Model{}, err
	}
	return entry.Model(), nil
}

// Model converts reported metadata into Aura's model representation.
func (entry Model) Model() model.Model {
	m := model.Model{Name: entry.ID, ParameterCount: model.ParseParameterName(entry.ID)}
	for _, n := range []int{entry.ContextLength, entry.ContextWindow, entry.Meta.ContextLength} {
		if n > 0 {
			m.ContextLength = model.ContextLength(n)
			break
		}
	}
	for name, cap := range map[string]capabilities.Capability{
		"function_calling": capabilities.Tools, "vision": capabilities.Vision,
		"reasoning": capabilities.Thinking,
	} {
		if supported, ok := entry.Capabilities[name]; ok {
			m.SetCapability(cap, supported)
		}
	}
	if !m.Knows(capabilities.Vision) && entry.Architecture.InputModalities != nil {
		m.SetCapability(capabilities.Vision, slices.Contains(entry.Architecture.InputModalities, "image"))
	}
	if slices.Contains(entry.SupportedParameters, "tools") && !m.Knows(capabilities.Tools) {
		m.SetCapability(capabilities.Tools, true)
	}
	if slices.Contains(entry.SupportedParameters, "reasoning_effort") {
		m.SetCapability(capabilities.Thinking, true)
		m.SetCapability(capabilities.ThinkingLevels, true)
	}
	return m
}
