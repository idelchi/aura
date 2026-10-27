package assistant

import "fmt"

// compactionBudget is the estimated input admitted for one summarization request
// (config.Compaction.InputBudget). It limits what is sent, never the summary.
type compactionBudget struct {
	// Tokens is the admitted estimated input; zero means no limit is known.
	Tokens int
	// Basis explains how Tokens was derived.
	Basis string
}

// IsKnown reports whether a limit could be derived.
func (b compactionBudget) IsKnown() bool {
	return b.Tokens > 0
}

// Admits reports whether a request with the given estimated input may be sent.
// Without a known limit every request is sent and provider overflow handling applies.
func (b compactionBudget) Admits(estimate int) bool {
	return !b.IsKnown() || estimate <= b.Tokens
}

// String describes the budget for debug logs.
func (b compactionBudget) String() string {
	if !b.IsKnown() {
		return "none (" + b.Basis + ")"
	}

	return fmt.Sprintf("%d (%s)", b.Tokens, b.Basis)
}
