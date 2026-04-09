package commands

import (
	"context"

	"github.com/idelchi/aura/internal/slash"
)

// Stats creates the /stats command to show session metrics.
func Stats() slash.Command {
	return slash.Command{
		Name:        "/stats",
		Description: "Show session statistics",
		Category:    "config",
		Execute: func(_ context.Context, c slash.Context, _ ...string) (string, error) {
			output := c.SessionStats().Display()

			output += "\n" + c.Status().ContextDisplay(c.Builder().Len())

			return output, nil
		},
	}
}
