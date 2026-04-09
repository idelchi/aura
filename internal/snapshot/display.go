package snapshot

import (
	"fmt"

	humanize "github.com/dustin/go-humanize"
)

// PickerLabel returns the label shown in the /undo picker.
// Format: "Turn 5 — refactor the auth module".
func (s Snapshot) PickerLabel(turnNumber int) string {
	msg := s.Message
	if len(msg) > 50 {
		msg = msg[:47] + "..."
	}

	return fmt.Sprintf("Turn %d — \"%s\"", turnNumber, msg)
}

// PickerDescription returns the secondary text (relative timestamp).
func (s Snapshot) PickerDescription() string {
	return humanize.Time(s.CreatedAt)
}
