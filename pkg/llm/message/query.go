package message

import (
	"slices"

	"github.com/idelchi/aura/pkg/llm/roles"
)

// ForLLM returns messages to send to the LLM provider.
// Excludes internal types (DisplayOnly, Bookmark, Metadata).
// Includes Normal, Synthetic (one-turn influence), Ephemeral (one-turn errors).
func (ms Messages) ForLLM() Messages {
	return ms.WithoutInternalMessages()
}

// ForCompaction returns messages to send to the compaction LLM.
// Excludes Synthetic, Ephemeral, internal types, and thinking content.
// Tool results are truncated to maxResultLen characters.
func (ms Messages) ForCompaction(maxResultLen int) Messages {
	return ms.WithoutSyntheticMessages().
		WithoutInternalMessages().
		WithoutEphemeralMessages().
		WithoutThinking().
		TruncateResults(maxResultLen)
}

// ForPreservation returns messages safe to keep after compaction.
// Only retains Normal messages — strips one-turn types (Synthetic, Ephemeral)
// and internal types (DisplayOnly, Bookmark, Metadata).
func (ms Messages) ForPreservation() Messages {
	return slices.DeleteFunc(slices.Clone(ms), func(msg Message) bool {
		return msg.Type != Normal
	})
}

// ForSave returns messages to persist in session files.
// Excludes Ephemeral (one-turn, paired with tool calls).
// Includes Normal, Synthetic, DisplayOnly, Bookmark, Metadata.
func (ms Messages) ForSave() Messages {
	return ms.WithoutEphemeralMessages()
}

// ForExport returns messages for human-readable export (plaintext, markdown).
// Excludes Synthetic, Ephemeral, Metadata.
// Includes Normal, DisplayOnly, Bookmark.
func (ms Messages) ForExport() Messages {
	return slices.DeleteFunc(slices.Clone(ms), func(msg Message) bool {
		return msg.IsSynthetic() || msg.IsEphemeral() || msg.IsMetadata()
	})
}

// ForExportStructured returns messages for machine-readable export (JSON, JSONL).
// Excludes Synthetic, Ephemeral.
// Includes Normal, DisplayOnly, Bookmark, Metadata.
func (ms Messages) ForExportStructured() Messages {
	return slices.DeleteFunc(slices.Clone(ms), func(msg Message) bool {
		return msg.IsSynthetic() || msg.IsEphemeral()
	})
}

// ForDisplay returns messages suitable for UI rendering on session restore.
// Excludes System role, Tool role, Metadata.
// Includes Normal (user/assistant), DisplayOnly, Bookmark.
func (ms Messages) ForDisplay() Messages {
	return slices.DeleteFunc(slices.Clone(ms), func(msg Message) bool {
		if msg.IsMetadata() {
			return true
		}

		return msg.Role == roles.System || msg.Role == roles.Tool
	})
}

// TokensForEstimation returns the token count of messages that affect context budget.
// Excludes internal types (tokens that never reach the LLM).
func (ms Messages) TokensForEstimation() int {
	total := 0

	for _, msg := range ms {
		if !msg.IsInternal() {
			total += msg.Tokens.Total
		}
	}

	return total
}
