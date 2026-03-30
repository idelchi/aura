// Package generation defines optional sampling, output, and thinking budget parameters
// for LLM requests. Shared between internal/config (agent YAML) and pkg/llm/request
// (provider calls), following the same cross-boundary pattern as pkg/llm/thinking.
package generation

// Generation holds optional generation parameters.
// Nil pointer = "don't send" — the provider uses its server-side default.
type Generation struct {
	Temperature      *float64 `yaml:"temperature"`
	TopP             *float64 `yaml:"top_p"`
	TopK             *int     `yaml:"top_k"`
	FrequencyPenalty *float64 `yaml:"frequency_penalty"`
	PresencePenalty  *float64 `yaml:"presence_penalty"`
	MaxOutputTokens  *int     `yaml:"max_output_tokens"`
	Stop             []string `yaml:"stop"`
	Seed             *int     `yaml:"seed"`
	ThinkBudget      *int     `yaml:"think_budget"`
}

// Merge applies non-nil fields from other onto g.
// Used for layering agent YAML values across inheritance chains
// without replacing fields the caller didn't set.
func (g *Generation) Merge(other *Generation) {
	if other == nil {
		return
	}

	if other.Temperature != nil {
		g.Temperature = other.Temperature
	}

	if other.TopP != nil {
		g.TopP = other.TopP
	}

	if other.TopK != nil {
		g.TopK = other.TopK
	}

	if other.FrequencyPenalty != nil {
		g.FrequencyPenalty = other.FrequencyPenalty
	}

	if other.PresencePenalty != nil {
		g.PresencePenalty = other.PresencePenalty
	}

	if other.MaxOutputTokens != nil {
		g.MaxOutputTokens = other.MaxOutputTokens
	}

	if other.Stop != nil {
		g.Stop = other.Stop
	}

	if other.Seed != nil {
		g.Seed = other.Seed
	}

	if other.ThinkBudget != nil {
		g.ThinkBudget = other.ThinkBudget
	}
}
