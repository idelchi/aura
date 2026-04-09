package config

// Compaction holds configuration for context compaction.
type Compaction struct {
	// Threshold is the context window fill percentage that triggers automatic compaction.
	Threshold float64 `yaml:"threshold"`
	// MaxTokens is an absolute token count that triggers compaction when exceeded.
	// Takes priority over Threshold — when set, percentage mode is not used.
	MaxTokens int `yaml:"max_tokens"`
	// TrimThreshold is the fill percentage at which duplicate synthetic messages are removed.
	TrimThreshold float64 `yaml:"trim_threshold"`
	// TrimMaxTokens is an absolute token count that triggers synthetic trimming when exceeded.
	// Takes priority over TrimThreshold — when set, percentage mode is not used.
	TrimMaxTokens int `yaml:"trim_max_tokens"`
	// KeepLastMessages is the number of recent messages preserved during compaction.
	KeepLastMessages int `yaml:"keep_last_messages"`
	// Agent is the name of the agent to use for compaction (empty = current provider).
	Agent string `yaml:"agent"`
	// Prompt overrides the compaction agent: use the current agent's model with this
	// named system prompt (from prompts/) instead of delegating to a dedicated agent.
	// When set, Agent is ignored. This is the "self-compact" signal.
	Prompt string `yaml:"prompt"`
	// ToolResultMaxLen is the max character length for tool results in the compaction transcript.
	ToolResultMaxLen int `yaml:"tool_result_max_length"`
	// TruncationRetries is the sequence of progressively lower tool result lengths tried
	// when compaction fails. The initial attempt uses ToolResultMaxLen; these are the retries.
	TruncationRetries []int `yaml:"truncation_retries"`
	// Chunks is the number of chunks to split the compactable history into.
	// 1 = single-pass (current behavior). >1 = sequential chunked compaction
	// where each chunk's summary feeds into the next.
	Chunks int `yaml:"chunks"`
	// Prune controls tool result pruning to keep context lean.
	Prune Prune `yaml:"prune"`
}

// Prune holds configuration for tool result pruning.
type Prune struct {
	// Mode controls when pruning runs: "off", "iteration", "compaction".
	Mode PruneMode `validate:"omitempty,oneof=off iteration compaction" yaml:"mode"`
	// ProtectPercent is the percentage of the main model's context to protect
	// from pruning (most recent messages).
	ProtectPercent float64 `yaml:"protect_percent"`
	// ArgThreshold is the minimum estimated token count for a tool call's
	// arguments to be eligible for pruning.
	ArgThreshold int `yaml:"arg_threshold"`
}

// PruneMode controls when tool result pruning runs.
type PruneMode string

const (
	PruneModeOff        PruneMode = "off"
	PruneModeIteration  PruneMode = "iteration"
	PruneModeCompaction PruneMode = "compaction"
)

// AtIteration reports whether pruning should run after each loop iteration.
func (m PruneMode) AtIteration() bool {
	return m == PruneModeIteration
}

// AtCompaction reports whether pruning should run during compaction.
func (m PruneMode) AtCompaction() bool {
	return m == PruneModeCompaction
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (c *Compaction) ApplyDefaults() error {
	if c.Threshold == 0 {
		c.Threshold = 80
	}

	if c.TrimThreshold == 0 {
		c.TrimThreshold = 50
	}

	if c.KeepLastMessages == 0 {
		c.KeepLastMessages = 10
	}

	if c.ToolResultMaxLen == 0 {
		c.ToolResultMaxLen = 200
	}

	if c.TruncationRetries == nil {
		c.TruncationRetries = []int{150, 100, 50, 0}
	}

	if c.Chunks <= 0 {
		c.Chunks = 1
	}

	return c.Prune.ApplyDefaults()
}

// ApplyDefaults sets sane defaults for zero-valued prune fields.
func (p *Prune) ApplyDefaults() error {
	if p.Mode == "" {
		p.Mode = PruneModeOff
	}

	if p.ProtectPercent == 0 {
		p.ProtectPercent = 30
	}

	if p.ArgThreshold == 0 {
		p.ArgThreshold = 200
	}

	return nil
}
