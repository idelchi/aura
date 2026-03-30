package commands

import (
	"context"

	"github.com/idelchi/aura/internal/slash"
)

// Reload creates the /reload command to hot-reload config from disk.
func Reload() slash.Command {
	return slash.Command{
		Name:        "/reload",
		Description: "Reload config from disk",
		Category:    "config",
		Execute: func(ctx context.Context, c slash.Context, _ ...string) (string, error) {
			if err := c.Reload(ctx); err != nil {
				return "", err
			}

			return "Config reloaded.", nil
		},
	}
}
