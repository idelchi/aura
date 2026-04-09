package commands

import (
	"context"

	"github.com/idelchi/aura/internal/slash"
)

// Ctx creates the /ctx command to show context stats.
func Ctx() slash.Command {
	return slash.Command{
		Name:        "/ctx",
		Aliases:     []string{"/context"},
		Description: "Show context window stats",
		Category:    "context",
		Execute: func(_ context.Context, c slash.Context, _ ...string) (string, error) {
			return c.Status().ContextDisplay(len(c.Builder().History())), nil
		},
	}
}
