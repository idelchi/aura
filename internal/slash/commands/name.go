package commands

import (
	"context"
	"errors"
	"strings"

	"github.com/idelchi/aura/internal/slash"
)

// Name creates the /name command to set or show the session name.
func Name() slash.Command {
	return slash.Command{
		Name:        "/name",
		Description: "Set or show the session name",
		Hints:       "[name]",
		Category:    "session",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if c.SessionManager() == nil {
				return "", errors.New("sessions not configured")
			}

			// No args: show current ID
			if len(args) == 0 {
				id := c.SessionManager().ActiveID()
				if id == "" {
					return "No active session", nil
				}

				return "Session: " + id, nil
			}

			name := strings.Join(args, "-")
			if name == "" {
				return "", errors.New("name cannot be empty")
			}

			if err := c.SessionManager().SetName(name); err != nil {
				return "", err
			}

			return "Session named: " + name, nil
		},
	}
}
