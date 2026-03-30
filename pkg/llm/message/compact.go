package message

import (
	"slices"

	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/truncate"
)

// Truncated returns a copy with ephemeral messages removed and tool results truncated.
func (ms Messages) Truncated(maxLen int) Messages {
	return ms.WithoutEphemeralMessages().TruncateResults(maxLen)
}

// TruncateResults truncates tool role message content to maxLen characters.
func (ms Messages) TruncateResults(maxLen int) Messages {
	result := slices.Clone(ms)

	for i, msg := range result {
		if msg.Role == roles.Tool && maxLen > 0 {
			result[i].Content = truncate.Truncate(msg.Content, maxLen)
		}
	}

	return result
}

// WithoutThinking returns a copy with thinking fields cleared from all messages.
func (ms Messages) WithoutThinking() Messages {
	result := slices.Clone(ms)

	for i := range result {
		result[i].Thinking = ""
		result[i].ThinkingSignature = ""
	}

	return result
}

// TrimDuplicateSynthetics keeps only the most recent occurrence of each synthetic message,
// removing older duplicates. Non-synthetic messages are always kept.
func (ms *Messages) TrimDuplicateSynthetics() {
	// Count occurrences of each synthetic content
	counts := make(map[string]int)

	for _, msg := range *ms {
		if msg.IsSynthetic() {
			counts[msg.Content]++
		}
	}

	// Delete all but the last occurrence (decrement count; delete while count > 0)
	*ms = slices.DeleteFunc(*ms, func(msg Message) bool {
		if !msg.IsSynthetic() {
			return false
		}

		counts[msg.Content]--

		return counts[msg.Content] > 0
	})
}
