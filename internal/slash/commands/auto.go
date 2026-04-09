package commands

import (
	"context"
	"errors"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/truthy"
)

// Auto creates the /auto command for toggling auto mode.
func Auto() slash.Command {
	return slash.Command{
		Name:        "/auto",
		Description: "Toggle auto mode (continue while TODOs are pending or in-progress)",
		Hints:       "[on|off]",
		Category:    "execution",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				return formatState("Auto", c.Resolved().Auto), nil
			}

			val, err := truthy.Parse(args[0])
			if err != nil {
				return "", errors.Join(err, slash.ErrUsage)
			}

			c.SetAuto(val)

			return formatState("Auto", val), nil
		},
	}
}
