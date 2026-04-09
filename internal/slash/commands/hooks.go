package commands

import (
	"context"

	"github.com/idelchi/aura/internal/hooks"
	"github.com/idelchi/aura/internal/slash"
)

// Hooks creates the /hooks command to display active hooks and execution order.
func Hooks() slash.Command {
	return slash.Command{
		Name:        "/hooks",
		Description: "Show active hooks and execution order",
		Category:    "config",
		Execute: func(_ context.Context, c slash.Context, _ ...string) (string, error) {
			r := c.Resolved()
			hks := c.Cfg().FilteredHooks(r.Agent, r.Mode)

			pre, post, err := hooks.Order(hks)
			if err != nil {
				return "", err
			}

			return hks.Display(pre, post), nil
		},
	}
}
