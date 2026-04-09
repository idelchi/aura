package message

import (
	"slices"

	"github.com/idelchi/aura/pkg/llm/roles"
)

// Messages is a collection of chat messages.
type Messages []Message

// New creates a Messages collection from the given messages.
func New(messages ...Message) Messages {
	return Messages(messages)
}

// Add appends a message to the collection.
func (ms *Messages) Add(msg Message) {
	*ms = append(*ms, msg)
}

// KeepFirstByRole retains only the first message with the specified role, removing all others.
func (ms *Messages) KeepFirstByRole(role roles.Role) {
	found := false

	*ms = slices.DeleteFunc(*ms, func(msg Message) bool {
		if msg.Role != role {
			return false
		}

		if !found {
			found = true

			return false
		}

		return true
	})
}

// Last returns a pointer to the last message, or nil if empty.
func (ms Messages) Last() *Message {
	if len(ms) == 0 {
		return nil
	}

	return &ms[len(ms)-1]
}

// TotalTokens returns the sum of Tokens.Total across all messages.
func (ms Messages) TotalTokens() int {
	total := 0

	for _, msg := range ms {
		total += msg.Tokens.Total
	}

	return total
}

// Clear removes all messages except the first (system prompt) from the collection.
// If only one message exists, it is preserved as-is.
func (ms *Messages) Clear() {
	if len(*ms) > 1 {
		*ms = Messages{(*ms)[0]}
	}
}
