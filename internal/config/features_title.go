package config

// Title holds configuration for session title generation.
type Title struct {
	// Disabled skips LLM title generation and uses the first user message instead.
	Disabled *bool `yaml:"disabled"`
	// Agent is the name of the agent to use for title generation.
	// If empty and not disabled, falls back to the current conversation's provider/model.
	Agent string `yaml:"agent"`
	// Prompt overrides the title agent: use the current agent's model with this
	// named system prompt (from prompts/) instead of delegating to a dedicated agent.
	// When set, Agent is ignored.
	Prompt string `yaml:"prompt"`
	// MaxLength is the maximum character length for generated titles.
	MaxLength int `yaml:"max_length"`
}

// IsDisabled returns true if LLM title generation is explicitly disabled.
func (t Title) IsDisabled() bool { return t.Disabled != nil && *t.Disabled }

// IsEnabled returns true if LLM title generation is active (not explicitly disabled).
func (t Title) IsEnabled() bool { return !t.IsDisabled() }

// ApplyDefaults sets sane defaults for zero-valued fields.
func (t *Title) ApplyDefaults() error {
	if t.MaxLength == 0 {
		t.MaxLength = 50
	}

	return nil
}
