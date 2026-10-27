package config

import (
	"fmt"
	"time"
)

// Compaction holds configuration for context compaction.
type Compaction struct {
	// Timeout bounds a complete compaction/recovery operation, including retries.
	// Zero adds no separate deadline; the caller's deadline still applies.
	Timeout time.Duration `yaml:"timeout"`
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
	// ToolResultMaxLen is the max byte length of each tool result in the compaction
	// transcript. Longer results are excerpted with a marker; zero or less omits them.
	ToolResultMaxLen int `yaml:"tool_result_max_length"`
	// TruncationRetries are progressively lower tool result lengths. A summarization
	// request uses the first length, starting with ToolResultMaxLen, whose estimated
	// input fits InputBudget, and moves on after a provider context overflow.
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

// InputBudget returns the estimated input admitted for one summarization request on a
// model with the given context window (0 when unknown) and optional output allowance.
// It applies this compaction trigger, so a summarization request is admitted under the
// same fill limit as any other request in the conversation:
//   - MaxTokens when set and smaller than a known window (or the window is unknown);
//   - otherwise Threshold percent of the window. This also replaces a MaxTokens that
//     reaches or exceeds a smaller summarizer window, which would leave no room for the
//     summary. A Threshold above 100 (automatic compaction effectively disabled) is
//     capped at the window and leaves no headroom; one at or below 0 is an error.
//
// A configured output allowance then stays free within a known window. Zero tokens
// means no limit can be derived (unknown window and no MaxTokens). The budget admits
// input only; it never limits the summary. basis explains the derivation.
func (c Compaction) InputBudget(window int, maxOutput *int) (tokens int, basis string, err error) {
	switch {
	case c.MaxTokens > 0 && (window == 0 || c.MaxTokens < window):
		tokens, basis = c.MaxTokens, fmt.Sprintf("compaction.max_tokens %d", c.MaxTokens)
	case window > 0:
		if c.Threshold <= 0 {
			return 0, "", fmt.Errorf(
				"compaction.threshold %g cannot derive an input budget for a %d-token summarizer window", c.Threshold, window)
		}

		tokens = int(float64(window) * c.Threshold / 100)
		basis = fmt.Sprintf("compaction.threshold %g%% of a %d-token window", c.Threshold, window)

		if c.MaxTokens > 0 {
			basis += fmt.Sprintf(" (compaction.max_tokens %d does not fit below it)", c.MaxTokens)
		}

		if tokens > window {
			tokens, basis = window, basis+", capped at the window"
		}
	default:
		return 0, "unknown context window and no compaction.max_tokens", nil
	}

	if window > 0 && maxOutput != nil && window-*maxOutput < tokens {
		tokens = window - *maxOutput
		basis = fmt.Sprintf("%d-token window minus max_output_tokens %d", window, *maxOutput)
	}

	if tokens <= 0 {
		return 0, basis, fmt.Errorf("compaction input budget is %d tokens (%s)", tokens, basis)
	}

	return tokens, basis, nil
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
	if err := c.ValidateResolved(); err != nil {
		return err
	}
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

// ValidateResolved checks values after agent, mode, task and CLI overlays.
func (c Compaction) ValidateResolved() error {
	if c.Timeout < 0 {
		return fmt.Errorf("compaction.timeout must be nonnegative")
	}
	return nil
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
