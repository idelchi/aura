package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/ui"
)

// Resume creates the /resume command to load a saved session.
func Resume() slash.Command {
	return slash.Command{
		Name:        "/resume",
		Aliases:     []string{"/load"},
		Description: "Resume a saved session or list sessions",
		Hints:       "[id]",
		Category:    "session",
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			if c.SessionManager() == nil {
				return "", errors.New("sessions not configured")
			}

			// No args: open session picker
			if len(args) == 0 {
				return listSessions(c)
			}

			id := args[0]

			// Try prefix match against session list
			resolved, err := c.SessionManager().Find(id)
			if err != nil {
				return "", err
			}

			sess, err := c.SessionManager().Resume(resolved)
			if err != nil {
				return "", fmt.Errorf("resuming session: %w", err)
			}

			warnings := c.ResumeSession(ctx, sess)

			result := "Resumed session: " + sess.ShortDisplay()

			if len(warnings) > 0 {
				result += " [" + strings.Join(warnings, "; ") + "]"
			}

			return result, nil
		},
	}
}

// listSessions opens an interactive picker overlay with all saved sessions.
func listSessions(c slash.Context) (string, error) {
	result, err := c.SessionManager().List()
	if err != nil {
		return "", err
	}

	if len(result.Summaries) == 0 {
		return "No saved sessions", nil
	}

	items := make([]ui.PickerItem, 0, len(result.Summaries))

	for _, s := range result.Summaries {
		items = append(items, ui.PickerItem{
			Label:       s.PickerLabel(),
			Description: s.PickerDescription(),
			Action:      ui.ResumeSession{SessionID: s.ID},
		})
	}

	c.EventChan() <- ui.PickerOpen{Title: "Resume session:", Items: items}

	return "", nil
}
