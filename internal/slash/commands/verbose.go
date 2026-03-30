package commands

import (
	"context"
	"errors"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/truthy"
)

// Verbose creates the /verbose command to toggle thinking block visibility in output.
func Verbose() slash.Command {
	return slash.Command{
		Name:        "/verbose",
		Aliases:     []string{"/v"},
		Description: "Toggle thinking block visibility in output",
		Hints:       "[on|off]",
		Category:    "agent",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				return formatState("Verbose", c.Resolved().Verbose), nil
			}

			val, err := truthy.Parse(args[0])
			if err != nil {
				return "", errors.Join(err, slash.ErrUsage)
			}

			c.SetVerbose(val)

			return formatState("Verbose", val), nil
		},
	}
}
