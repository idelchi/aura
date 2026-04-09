package cachecmd

import (
	"context"
	"fmt"

	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/pkg/cache"
)

// Command creates the 'aura cache' command.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:        "cache",
		Usage:       "Manage the cache",
		Description: "Inspect or clear the cache directory used for provider metadata and model lists.",
		Commands: []*cli.Command{
			cleanCommand(flags),
		},
	}
}

func cleanCommand(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:        "clean",
		Usage:       "Delete the entire cache directory",
		Description: "Removes all cached data (catwalk metadata, model lists). Data will be re-fetched on next use.",
		Action: func(_ context.Context, _ *cli.Command) error {
			c := cache.New(flags.WriteHome(), false)

			if err := c.Clean(); err != nil {
				return fmt.Errorf("cleaning cache: %w", err)
			}

			fmt.Println("Cache cleaned.")

			return nil
		},
	}
}
