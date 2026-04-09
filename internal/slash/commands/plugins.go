package commands

import (
	"context"

	"github.com/idelchi/aura/internal/slash"
)

// Plugins creates the /plugins command to list loaded plugins and their capabilities.
func Plugins() slash.Command {
	return slash.Command{
		Name:        "/plugins",
		Description: "List loaded plugins and their capabilities",
		Category:    "tools",
		Execute: func(_ context.Context, c slash.Context, _ ...string) (string, error) {
			return c.PluginSummary(), nil
		},
	}
}
