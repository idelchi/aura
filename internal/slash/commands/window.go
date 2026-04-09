package commands

import (
	"context"
	"fmt"
	"strconv"

	"github.com/idelchi/aura/internal/slash"
)

// Window creates the /window command to set the context window size.
func Window() slash.Command {
	return slash.Command{
		Name:        "/window",
		Description: "Show or set context window size in tokens",
		Hints:       "[size]",
		Category:    "context",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				return c.Status().WindowDisplay(), nil
			}

			size, err := strconv.Atoi(args[0])
			if err != nil {
				return "", fmt.Errorf("invalid size %q: %w", args[0], slash.ErrUsage)
			}

			if err := c.ResizeContext(size); err != nil {
				return "", err
			}

			return fmt.Sprintf("Context window set to %d tokens", size), nil
		},
	}
}
