// Package roles defines conversation message roles.
package roles

// Role represents the role of a message.
type Role string

const (
	// User signifies a message from the user.
	User Role = "user"
	// Assistant signifies a message from the assistant.
	Assistant Role = "assistant"
	// System signifies a system message.
	System Role = "system"
	// Tool signifies a message from a tool result.
	Tool Role = "tool"
	// Synthetic signifies a synthetic message.
	Synthetic Role = "synthetic"
)

// String returns the string representation of the Role.
func (r Role) String() string {
	return string(r)
}
