package sdk

import "fmt"

// ValidateTransformed checks basic structural invariants on a transformed message array.
// Returns an error describing the first violation found, or nil if valid.
func ValidateTransformed(msgs []Message) error {
	if len(msgs) == 0 {
		return fmt.Errorf("transformed array is empty")
	}

	if msgs[0].Role != "system" {
		return fmt.Errorf("first message must be role=system, got %q", msgs[0].Role)
	}

	// Build set of tool call IDs from preceding assistant messages.
	callIDs := make(map[string]bool)

	for _, msg := range msgs {
		for _, tc := range msg.ToolCalls {
			callIDs[tc.ID] = true
		}

		if msg.Role == "tool" && msg.ToolCallID != "" {
			if !callIDs[msg.ToolCallID] {
				return fmt.Errorf("orphaned tool result: ToolCallID=%q has no matching tool call", msg.ToolCallID)
			}
		}
	}

	return nil
}
