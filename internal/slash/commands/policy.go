package commands

import (
	"context"

	"github.com/idelchi/aura/internal/slash"
)

// Policy creates the /policy command to display the effective tool policy.
func Policy() slash.Command {
	return slash.Command{
		Name:        "/policy",
		Description: "Show effective tool policy",
		Category:    "tools",
		Execute: func(_ context.Context, c slash.Context, _ ...string) (string, error) {
			r := c.Resolved()

			return c.ToolPolicy().PolicyDisplay(r.Agent, r.Mode), nil
		},
	}
}
