package message

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/truncate"
)

// truncatedPrefix is the sentinel prefix for truncated argument values.
// Used to detect already-pruned arguments without introducing fake parameter names.
const truncatedPrefix = "[truncated"

// PruneToolResults returns a copy with old tool results and large tool call arguments
// replaced by short placeholders. Walks backward, accumulating token distance from
// the end — messages beyond protectTokens are pruned.
func (ms Messages) PruneToolResults(protectTokens, argThreshold int, estimate func(string) int) Messages {
	result := slices.Clone(ms)
	pruneToolResults(result, protectTokens, argThreshold, estimate)

	return result
}

// PruneToolResultsInPlace is the mutating variant of PruneToolResults.
func (ms Messages) PruneToolResultsInPlace(protectTokens, argThreshold int, estimate func(string) int) {
	pruneToolResults(ms, protectTokens, argThreshold, estimate)
}

func pruneToolResults(msgs []Message, protectTokens, argThreshold int, estimate func(string) int) {
	var accumulated int

	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]

		// Accumulate BEFORE the prune check — the current message's tokens
		// count toward the protect window. This ensures the boundary message
		// itself is protected.
		accumulated += msg.Tokens.Total

		if accumulated < protectTokens {
			continue
		}

		switch {
		case msg.Role == roles.Tool && !isAlreadyPruned(msg.Content):
			preview := truncate.Truncate(msg.Content, 100)
			pruned := fmt.Sprintf(
				"[tool output pruned — was ~%d tokens]\n%s",
				msg.Tokens.Total, preview,
			)

			msgs[i].Content = pruned
			msgs[i].Tokens = Tokens{Total: estimate(pruned)}

		case msg.Role == roles.Assistant && len(msg.Calls) > 0:
			calls := slices.Clone(msg.Calls)
			pruned := false

			for j := range calls {
				if isArgsTruncated(calls[j].Arguments) {
					continue // already pruned
				}

				argJSON := truncate.MapToJSON(calls[j].Arguments)
				argTokens := estimate(argJSON)

				if argTokens > argThreshold {
					calls[j].Arguments = truncateArgs(calls[j].Arguments, argTokens)
					pruned = true
				}
			}

			if pruned {
				msgs[i].Calls = calls

				// Recalculate using 31a breakdown if available.
				// Content + Thinking are preserved, only args changed.
				contentTokens := msg.Tokens.Content
				thinkingTokens := msg.Tokens.Thinking

				if contentTokens == 0 && thinkingTokens == 0 && (msg.Content != "" || msg.Thinking != "") {
					// No 31a breakdown — fall back to re-estimation from text.
					// Without this, thinking tokens are invisible and Total drops
					// to just the pruned args estimate.
					contentTokens = estimate(msg.Content)
					thinkingTokens = estimate(msg.Thinking)
				}

				msgs[i].Tokens = Tokens{
					Total: contentTokens + thinkingTokens + estimateCallArgs(calls, estimate),
				}
			}
		}
	}
}

// truncateArgs truncates each argument value individually, preserving parameter names.
// This prevents models from learning fake parameter names (like "_pruned") from their
// own conversation history.
func truncateArgs(args map[string]any, totalTokens int) map[string]any {
	result := make(map[string]any, len(args))

	for k, v := range args {
		b, err := json.Marshal(v)
		if err != nil {
			result[k] = fmt.Sprintf("%s — was ~%d tokens]", truncatedPrefix, totalTokens)

			continue
		}

		valStr := string(b)

		// Truncate values longer than 100 bytes, preserving short ones exactly.
		if len(valStr) > 100 {
			preview := truncate.Truncate(valStr, 100)

			result[k] = fmt.Sprintf("%s — was ~%d tokens] %s", truncatedPrefix, len(b), preview)
		} else {
			result[k] = v
		}
	}

	return result
}

// isArgsTruncated checks if any argument value has already been truncated.
func isArgsTruncated(args map[string]any) bool {
	for _, v := range args {
		if s, ok := v.(string); ok && strings.HasPrefix(s, truncatedPrefix) {
			return true
		}
	}

	return false
}

// estimateCallArgs estimates total tokens for all tool call arguments.
func estimateCallArgs(calls []call.Call, estimate func(string) int) int {
	total := 0

	for _, c := range calls {
		total += estimate(truncate.MapToJSON(c.Arguments))
	}

	return total
}

// isAlreadyPruned checks if content was already replaced by a prune placeholder.
func isAlreadyPruned(content string) bool {
	return strings.HasPrefix(content, "[tool output pruned")
}
