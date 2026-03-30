package commands

import (
	"context"
	"fmt"
	"slices"

	humanize "github.com/dustin/go-humanize"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// Undo creates the /undo command for reverting to a previous turn.
func Undo() slash.Command {
	return slash.Command{
		Name:        "/undo",
		Aliases:     []string{"/rewind"},
		Description: "Rewind code and/or messages to a previous turn",
		Category:    "context",
		Silent:      true,
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			mgr := c.SnapshotManager()

			// No git — derive turn points from conversation history for message-only rewind.
			if mgr == nil {
				return undoFromHistory(c)
			}

			snapshots, err := mgr.List()
			if err != nil || len(snapshots) == 0 {
				return undoFromHistory(c)
			}

			// Build picker items — newest first for the dropdown
			items := make([]ui.PickerItem, 0, len(snapshots))

			for i := len(snapshots) - 1; i >= 0; i-- {
				s := snapshots[i]
				turnNum := i + 1

				items = append(items, ui.PickerItem{
					Label:       s.PickerLabel(turnNum),
					Description: s.PickerDescription(),
					Action: ui.UndoSnapshot{
						Hash:         s.Hash,
						MessageIndex: s.MessageIndex,
					},
				})
			}

			c.EventChan() <- ui.PickerOpen{Title: "Rewind to:", Items: items}

			return "", nil
		},
	}
}

// undoFromHistory builds an undo picker from conversation history when no git repo is available.
// Only message-only rewind is supported — no code restore.
func undoFromHistory(c slash.Context) (string, error) {
	history := c.Builder().History()

	var items []ui.PickerItem

	turnNum := 0

	for i, msg := range history {
		if msg.IsInternal() || msg.Role != roles.User {
			continue
		}

		turnNum++

		label := msg.Content
		if len(label) > 47 {
			label = label[:47] + "..."
		}

		var desc string

		if !msg.CreatedAt.IsZero() {
			desc = humanize.Time(msg.CreatedAt)
		}

		items = append(items, ui.PickerItem{
			Label:       fmt.Sprintf("Turn %d — \"%s\"", turnNum, label),
			Description: desc,
			Action: ui.UndoSnapshot{
				Hash:         "",
				MessageIndex: i,
			},
		})
	}

	if len(items) == 0 {
		return "Nothing to rewind", nil
	}

	slices.Reverse(items)

	c.EventChan() <- ui.PickerOpen{Title: "Rewind to:", Items: items}

	return "", nil
}
