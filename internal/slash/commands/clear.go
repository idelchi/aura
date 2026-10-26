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
		Rebinds:     true,
		Aliases:     []string{"/new"},
		Description: "Start a new conversation (the current session stays saved)",
		Category:    "session",
		Execute: func(ctx context.Context, c slash.Context, _ ...string) (string, error) {
			if err := c.NewSession(ctx); err != nil {
				return "", err
			}

			if err := c.ClearSnapshots(); err != nil {
				debug.Log("[snapshot] prune: %v", err)
			}

			c.EventChan() <- ui.CommandResult{Clear: true}

			c.EventChan() <- ui.StatusChanged{Status: c.Status()}

			c.EventChan() <- ui.DisplayHintsChanged{Hints: c.DisplayHints()}

			return "", nil
		},
	}
}
