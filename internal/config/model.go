package config

import (
	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// Model represents the configuration for an AI model.
type Model struct {
	// Name is the model identifier.
	Name string `validate:"required"`
	// Provider is the name of the provider hosting the model.
	Provider string `validate:"required"`
	// Think configures extended thinking mode (zero value=off, bool, or "low"/"medium"/"high").
	Think thinking.Value
	// Context is the maximum context window size.
	Context int
	// Generation holds optional sampling, output, and thinking budget parameters.
	Generation *generation.Generation
}
