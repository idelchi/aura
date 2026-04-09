package commands

import (
	"context"
	"errors"
	"slices"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/truthy"
)

// Done creates the /done command for toggling the Done tool.
func Done() slash.Command {
	return slash.Command{
		Name:        "/done",
		Description: "Toggle Done tool (explicit task completion signal)",
		Hints:       "[on|off]",
		Category:    "execution",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				return formatState("Done", c.Resolved().Done), nil
			}

			val, err := truthy.Parse(args[0])
			if err != nil {
				return "", errors.Join(err, slash.ErrUsage)
			}

			if err := c.SetDone(val); err != nil {
				return "", err
			}

			if val && !slices.Contains(c.ToolNames(), "Done") {
				// Revert — don't leave the flag on when the tool is filtered out.
				if err := c.SetDone(false); err != nil {
					return "", err
				}

				return "Done tool is disabled for this agent.", nil
			}

			return formatState("Done", val), nil
		},
	}
}
