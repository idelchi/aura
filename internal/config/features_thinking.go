package config

// Thinking holds configuration for thinking block management (strip and rewrite modes).
type Thinking struct {
	// Agent is the name of the agent to use for thinking rewrite (empty = current provider).
	Agent string `yaml:"agent"`
	// Prompt overrides the thinking agent: use the current agent's model with this
	// named system prompt (from prompts/) instead of delegating to a dedicated agent.
	// When set, Agent is ignored. This is the "self-rewrite" signal.
	Prompt string `yaml:"prompt"`
	// KeepLast is the number of recent messages whose thinking is preserved unchanged.
	KeepLast int `yaml:"keep_last"`
	// TokenThreshold is the minimum token count for a thinking block to be stripped or rewritten.
	TokenThreshold int `yaml:"token_threshold"`
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (t *Thinking) ApplyDefaults() error {
	if t.KeepLast == 0 {
		t.KeepLast = 5
	}

	if t.TokenThreshold == 0 {
		t.TokenThreshold = 300
	}

	return nil
}
