package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/slash"
)

// Fork creates the /fork command to save the current session as a new branch.
func Fork() slash.Command {
	return slash.Command{
		Name:        "/fork",
		Description: "Fork current session into a new one",
		Hints:       "[title]",
		Category:    "session",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if c.SessionManager() == nil {
				return "", errors.New("sessions not configured")
			}

			title := strings.Join(args, " ")
			if title == "" {
				if existing := c.SessionManager().ActiveTitle(); existing != "" {
					title = "Fork of " + existing
				}
			}

			// Clear active session so Save creates a new one
			c.SessionManager().Fork()

			meta := c.SessionMeta()

			saved, err := c.SessionManager().Save(title, meta, c.Builder().History(), c.TodoList())
			if err != nil {
				return "", fmt.Errorf("forking session: %w", err)
			}

			return "Forked session: " + saved.ShortDisplay(), nil
		},
	}
}
