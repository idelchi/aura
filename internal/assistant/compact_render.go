package assistant

import (
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/idelchi/aura/internal/injector"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/truncate"
)

// receiptExcerptBytes bounds each current-turn receipt. It is fixed, independent of
// the summarizer's tool-result setting: receipts stay in the rebuilt context for the
// rest of the turn, so their size must not grow with how much evidence the
// summarizer is allowed to read.
const receiptExcerptBytes = 200

// compactionReceipts retains actual current-turn tool responses outside the
// model-generated summary. They describe execution, never inferred side effects.
func compactionReceipts(calls []injector.ToolCall) string {
	var result strings.Builder
	result.WriteString("\n[CURRENT TURN TOOL RECEIPTS — recorded by Aura, not the summarizing model]\nThese are all tool invocations in this turn, not earlier turns. Prefer these receipts over conflicting summary claims. Execution alone does not prove an external effect.\n")
	if len(calls) == 0 {
		result.WriteString("No tools were invoked in this turn.\n")
	}
	for i, call := range calls {
		output := call.Result
		if call.Error != "" {
			output = "Execution failed: " + call.Error
		}
		marker := ""
		if shown := truncate.Prefix(output, receiptExcerptBytes); len(shown) < len(output) {
			marker = fmt.Sprintf(" [receipt excerpt: first %d of %d bytes; the rest is not shown]", len(shown), len(output))
			output = shown
		}
		fmt.Fprintf(&result, "%d. %s returned: %q%s\n", i+1, call.Name, output, marker)
	}
	result.WriteString("[END CURRENT TURN TOOL RECEIPTS]")
	return result.String()
}

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
	msgs := history[1:].WithoutInternalMessages()
	if keepLast == 0 {
		return msgs, nil
	}

	if len(msgs) <= keepLast {
		return nil, msgs.ForPreservation()
	}

	splitIdx := len(msgs) - keepLast

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
// It strips synthetic messages and thinking, bounds tool results with explicit
// excerpt/omission markers, and wraps the result in the summarizer's system prompt.
// User and assistant text is never shortened.
func prepareCompactionMessages(systemPrompt string, msgs message.Messages, maxLen int) message.Messages {
	result := message.New(message.Message{Role: roles.System, Content: systemPrompt})

	for _, msg := range msgs.ForCompaction(maxLen) {
		if msg.IsSystem() {
			continue
		}

		result = append(result, msg)
	}

	return result
}

// compactionRules returns the instruction shared by single-pass and chunked summaries:
// keep to what the supplied transcript establishes. Tool results in that transcript may
// be excerpts or omissions (message.ExcerptResults), so the rules forbid reconstructing
// them. They ask for faithfulness; they cannot guarantee it.
func compactionRules() string {
	return heredoc.Doc(`
	Rules:
	- Retain requirements, constraints, decisions and corrections only as the messages above state them. A later correction replaces what it corrects.
	- Copy an identifier, path, value, count or quotation only when it appears in full above. Put quotation marks only around text copied exactly.
	- Never complete or reconstruct a schema, listing, file, code, JSON value, quotation or catalogue from memory or partial evidence.
	- A tool result marked as an excerpt or as omitted is incomplete: name the source to re-read instead of describing its missing part.
	- Report a change, check or outcome as done only when a message above shows it.
	- Follow the system instructions for what to retain. Do not continue the task or call tools; write only the summary.
	`)
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

	userPrompt := "Summarize the conversation above for its continuation.\n\n" + compactionRules()

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
		"This is chunk %d of %d. Write one summary that combines the previous summary, if any, with the messages above. "+
			"The previous summary is itself a summary: keep what it says unless these messages correct it, and add no detail that neither contains.\n\n%s",
		chunkNum, totalChunks, compactionRules(),
	))

	result = append(result, message.Message{Role: roles.User, Content: strings.Join(parts, "\n\n")})

	return result
}
