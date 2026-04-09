package commands

import (
	"context"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/ui"
)

// Clear creates the /clear command to clear conversation history and reset the UI.
func Clear() slash.Command {
	return slash.Command{
		Name:        "/clear",
		Aliases:     []string{"/new"},
		Description: "Clear conversation history",
		Category:    "session",
		Execute: func(ctx context.Context, c slash.Context, _ ...string) (string, error) {
			c.Builder().Clear()
			c.Builder().UpdateSystemPrompt(ctx, c.SystemPrompt())
			c.ResetTokens()

			if mgr := c.SnapshotManager(); mgr != nil {
				if err := mgr.Prune(); err != nil {
					debug.Log("[snapshot] prune: %v", err)
				}
			}

			c.EventChan() <- ui.CommandResult{Clear: true}

			c.EventChan() <- ui.StatusChanged{Status: c.Status()}

			c.EventChan() <- ui.DisplayHintsChanged{Hints: c.DisplayHints()}

			return "", nil
		},
	}
}
