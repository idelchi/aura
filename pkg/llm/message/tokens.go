package message

import "fmt"

// Tokens holds token counts for a message.
type Tokens struct {
	// Total is the overall token count for this message.
	// Assistant: exact from usageData.Output (API-reported).
	// Tool: delta-backfilled from API usage (exact).
	// User: estimated at creation time.
	Total int `json:"total,omitempty"`
	// Content is the estimated token count for the message content.
	// Populated via provider tokenizer (estimateTokens). Assistant messages only.
	Content int `json:"content,omitempty"`
	// Thinking is the estimated token count for the thinking/reasoning content.
	// Populated via provider tokenizer (estimateTokens). Assistant messages only.
	Thinking int `json:"thinking,omitempty"`
	// Tools is Total - Content - Thinking. Assistant messages only.
	// Captures the actual cost of tool call JSON, argument encoding, special tokens.
	// Derived as the exact remainder — avoids estimating structured JSON where estimation is worst.
	Tools int `json:"tools,omitempty"`
}

// String returns a compact summary of token counts.
func (t Tokens) String() string {
	if t.Content > 0 || t.Thinking > 0 || t.Tools > 0 {
		return fmt.Sprintf("total=%d content=%d thinking=%d tools=%d", t.Total, t.Content, t.Thinking, t.Tools)
	}

	return fmt.Sprintf("total=%d", t.Total)
}
