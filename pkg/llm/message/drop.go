package message

import (
	"slices"

	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// DropN removes the last n messages from the collection.
func (ms *Messages) DropN(n int) {
	if n >= len(*ms) {
		*ms = Messages{}
	} else if n > 0 {
		*ms = (*ms)[:len(*ms)-n]
	}
}

// WithoutSyntheticMessages returns a copy of the Messages without synthetic messages.
func (ms Messages) WithoutSyntheticMessages() Messages {
	return slices.DeleteFunc(slices.Clone(ms), func(msg Message) bool {
		return msg.IsSynthetic()
	})
}

// WithoutEphemeralMessages returns a copy of the Messages without ephemeral messages.
func (ms Messages) WithoutEphemeralMessages() Messages {
	return slices.DeleteFunc(slices.Clone(ms), func(msg Message) bool {
		return msg.IsEphemeral()
	})
}

// EjectEphemeralMessages removes ephemeral tool result messages and their matching
// calls from assistant messages. This is paired removal — both the tool result and
// the call that triggered it are stripped, preventing orphaned tool results that
// Anthropic/Google would reject.
func (ms *Messages) EjectEphemeralMessages() {
	// 1. Collect ToolCallIDs from ephemeral tool result messages.
	ephemeralIDs := map[string]bool{}

	for _, msg := range *ms {
		if msg.IsEphemeral() {
			ephemeralIDs[msg.ToolCallID] = true
		}
	}

	if len(ephemeralIDs) == 0 {
		return
	}

	// 2. Remove ephemeral tool result messages.
	*ms = slices.DeleteFunc(*ms, func(msg Message) bool {
		return msg.IsEphemeral()
	})

	// 3. Strip matching calls from assistant messages.
	for i := range *ms {
		msg := &(*ms)[i]
		if !msg.IsAssistant() || len(msg.Calls) == 0 {
			continue
		}

		msg.Calls = slices.DeleteFunc(msg.Calls, func(c call.Call) bool {
			return ephemeralIDs[c.ID]
		})
	}

	// 4. Remove assistant messages that became empty (no calls, no content).
	*ms = slices.DeleteFunc(*ms, func(msg Message) bool {
		return msg.IsAssistant() && len(msg.Calls) == 0 && msg.Content == ""
	})
}

// WithoutInternalMessages returns a copy of the Messages without internal messages
// (DisplayOnly, Bookmark, Metadata) — types that should never reach any LLM.
func (ms Messages) WithoutInternalMessages() Messages {
	return slices.DeleteFunc(slices.Clone(ms), func(msg Message) bool {
		return msg.IsInternal()
	})
}

// PruneEmptyAssistantMessages removes assistant messages with no content, thinking, or tool calls.
func (ms *Messages) PruneEmptyAssistantMessages() {
	*ms = slices.DeleteFunc(*ms, func(msg Message) bool {
		return msg.IsAssistant() && msg.IsEmpty()
	})
}
