package message

import (
	"fmt"
	"slices"

	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/truncate"
)

// Truncated returns a copy with ephemeral messages removed and tool results truncated.
func (ms Messages) Truncated(maxLen int) Messages {
	return ms.WithoutEphemeralMessages().TruncateResults(maxLen)
}

// TruncateResults truncates tool role message content to maxLen bytes with a bare "..." suffix.
// Compaction transcripts use ExcerptResults, which states what was cut.
func (ms Messages) TruncateResults(maxLen int) Messages {
	result := slices.Clone(ms)

	for i, msg := range result {
		if msg.Role == roles.Tool && maxLen > 0 {
			result[i].Content = truncate.Truncate(msg.Content, maxLen)
		}
	}

	return result
}

// ExcerptResults bounds tool results for a summarizing transcript and states
// exactly what was left out. A result of at most limit bytes is unchanged. A
// longer result keeps its longest UTF-8-safe prefix of at most limit bytes,
// followed by a marker with the shown and original byte counts. A limit of zero
// or less omits every non-empty result and leaves only a marker with its size.
// The marker says the rest is absent, so an excerpt is never presented as a
// complete document, JSON value or listing. Unlike TruncateResults, the cut is
// never silent.
func (ms Messages) ExcerptResults(limit int) Messages {
	result := slices.Clone(ms)

	for i, msg := range result {
		if msg.Role != roles.Tool || msg.Content == "" || (limit > 0 && len(msg.Content) <= limit) {
			continue
		}

		original := len(msg.Content)
		shown := truncate.Prefix(msg.Content, limit)

		if shown == "" {
			result[i].Content = fmt.Sprintf(
				"[tool result omitted: all %d bytes are absent from this transcript]", original)

			continue
		}

		result[i].Content = fmt.Sprintf(
			"%s\n[tool result excerpt: first %d of %d bytes shown; the remaining %d bytes are absent from this transcript]",
			shown, len(shown), original, original-len(shown))
	}

	return result
}

// ResultsBeyond counts the tool results that ExcerptResults(limit) would cut or omit.
func (ms Messages) ResultsBeyond(limit int) int {
	count := 0

	for _, msg := range ms {
		if msg.Role == roles.Tool && msg.Content != "" && (limit <= 0 || len(msg.Content) > limit) {
			count++
		}
	}

	return count
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
