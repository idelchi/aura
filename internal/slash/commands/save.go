package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/slash"
)

// Save creates the /save command to persist the current session.
func Save() slash.Command {
	return slash.Command{
		Name:        "/save",
		Description: "Save current session",
		Hints:       "[title]",
		Category:    "session",
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			if c.SessionManager() == nil {
				return "", errors.New("sessions not configured")
			}

			title := strings.Join(args, " ")
			if title == "" {
				title = c.SessionManager().ActiveTitle()
			}

			// Auto-generate title if still empty
			if title == "" {
				generated, err := c.GenerateTitle(ctx)
				if err == nil {
					title = generated
				}
			}

			meta := c.SessionMeta()

			saved, err := c.SessionManager().Save(title, meta, c.Builder().History(), c.TodoList())
			if err != nil {
				return "", fmt.Errorf("saving session: %w", err)
			}

			return "Saved session: " + saved.ShortDisplay(), nil
		},
	}
}
