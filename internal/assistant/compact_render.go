package assistant

import (
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// splitHistory divides API history into (toCompact, preserved).
// System prompt (index 0) is excluded from both — it stays in place via Builder.
// Preserved messages are stripped to Normal-only via ForPreservation().
// Internal types (DisplayOnly, Bookmark, Metadata) don't count toward keepLast.
func splitHistory(history message.Messages, keepLast int) (toCompact, preserved message.Messages) {
	if len(history) <= 1 {
		return nil, nil
	}

	if keepLast < 0 {
		keepLast = 0
	}

	// Skip system prompt
	msgs := history[1:]

	// Count non-internal messages to see if we have enough to split.
	nonInternal := 0

	for _, msg := range msgs {
		if !msg.IsInternal() {
			nonInternal++
		}
	}

	if nonInternal <= keepLast {
		return nil, msgs.ForPreservation()
	}

	// Backward walk skipping internal types to find split point.
	splitIdx := len(msgs) // default: compact everything
	kept := 0

	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].IsInternal() {
			continue
		}

		kept++

		if kept >= keepLast {
			splitIdx = i

			break
		}
	}

	// Adjust split boundary for tool call/result pair integrity.
	// If the first preserved message is a Tool result, move it (and any preceding
	// Tool results) into the preserved set along with the preceding Assistant message.
	// When keepLast=0, splitIdx == len(msgs) — compact everything, no boundary adjustment needed.
	if splitIdx < len(msgs) {
		for splitIdx > 0 && msgs[splitIdx].IsTool() {
			splitIdx--
		}
	}

	// If we walked past everything, nothing to compact
	if splitIdx <= 0 {
		return nil, msgs.ForPreservation()
	}

	return msgs[:splitIdx], msgs[splitIdx:].ForPreservation()
}

// wrapCompactionSummary wraps the compaction summary in markers with optional todo state.
func wrapCompactionSummary(summary, todoState string, pendingCount int) string {
	var b strings.Builder

	b.WriteString(heredoc.Doc(`
		*** [COMPACTION]:
		Context has been compacted to fit within the model's limits. Here is the summarized context:

	`))
	b.WriteString(summary)

	if trimmed := strings.TrimSpace(todoState); trimmed != "" {
		b.WriteString("\n\nHere are the current TODOS:\n")
		b.WriteString(trimmed)

		if pendingCount > 1 {
			b.WriteString(
				"\n\nNote: There are multiple pending ToDos. You MUST review now if you have already accomplished them and if they can be marked as completed.",
			)
		}
	}

	b.WriteString("\n*** [END COMPACTION]")

	return b.String()
}

// splitIntoChunks divides messages into n roughly-equal chunks,
// never splitting a tool-call/tool-result pair across chunk boundaries.
func splitIntoChunks(msgs message.Messages, n int) []message.Messages {
	target := max(len(msgs)/n, 1)

	var chunks []message.Messages

	start := 0

	for i := 0; i < n-1 && start < len(msgs); i++ {
		end := min(start+target, len(msgs))

		// Walk forward past any tool results to keep pairs together
		for end < len(msgs) && msgs[end].IsTool() {
			end++
		}

		if end > len(msgs) {
			end = len(msgs)
		}

		chunks = append(chunks, msgs[start:end])
		start = end
	}

	// Last chunk gets the remainder
	if start < len(msgs) {
		chunks = append(chunks, msgs[start:])
	}

	return chunks
}

// prepareCompactionMessages filters and preprocesses conversation messages for compaction.
// It strips synthetic messages, thinking, truncates tool results, and wraps in a system message.
func prepareCompactionMessages(systemPrompt string, msgs message.Messages, maxLen int) message.Messages {
	prepared := msgs.ForCompaction(maxLen)

	result := message.New(message.Message{Role: roles.System, Content: systemPrompt})

	for _, msg := range prepared {
		if msg.IsSystem() {
			continue
		}

		if msg.IsTool() && maxLen <= 0 {
			msg.Content = "[tool output stripped]"
		}

		result = append(result, msg)
	}

	return result
}

// buildCompactionMessages builds a multi-message request with the actual
// preprocessed conversation history instead of a rendered text transcript.
// The model sees structured tool calls/results as native messages, not flattened text.
func buildCompactionMessages(
	systemPrompt string,
	toCompact message.Messages,
	maxLen int,
	todoState string,
) message.Messages {
	result := prepareCompactionMessages(systemPrompt, toCompact, maxLen)

	userPrompt := heredoc.Doc(`
		Produce a compacted summary of the conversation above. Preserve the following EXACTLY:
		- Requirement checklists, acceptance criteria, and structural constraints (reproduce verbatim)
		- Specific file paths, package names, and dependency choices that were decided or required
		- Decisions made and their rationale — only report decisions that were EXPLICITLY stated, not inferred from errors

		Summarize narrative and discussion freely, but structured data must survive intact.
		Do NOT attempt to continue the task or issue tool calls. Only SUMMARIZE as instructed.
	`)

	if todoState != "" {
		userPrompt = todoState + "\n\n" + userPrompt
	}

	result = append(result, message.Message{Role: roles.User, Content: userPrompt})

	return result
}

// buildChunkMessages builds a multi-message request for a single chunk
// with the actual preprocessed conversation history.
func buildChunkMessages(
	systemPrompt string,
	chunk message.Messages,
	maxLen int,
	todoState, prevSummary string,
	chunkNum, totalChunks int,
) message.Messages {
	result := prepareCompactionMessages(systemPrompt, chunk, maxLen)

	var parts []string

	if prevSummary != "" {
		parts = append(parts, heredoc.Docf(`
			<BEGIN PREVIOUS SUMMARY>
			This is the summary of chunks 1-%d (of %d total):
			%s
			<END PREVIOUS SUMMARY>`, chunkNum-1, totalChunks, prevSummary))
	}

	if todoState != "" {
		parts = append(parts, todoState)
	}

	parts = append(parts, fmt.Sprintf(
		"This is chunk %d of %d. Produce a compacted summary that incorporates any previous summary and this chunk. "+
			"Preserve requirement checklists, structural constraints, and explicit decisions verbatim. "+
			"Do NOT attempt to continue the task or issue tool calls. Only SUMMARIZE as instructed.",
		chunkNum, totalChunks,
	))

	result = append(result, message.Message{Role: roles.User, Content: strings.Join(parts, "\n\n")})

	return result
}
