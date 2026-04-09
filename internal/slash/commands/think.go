package commands

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/truthy"
)

// Think creates the /think command to set thinking mode.
func Think() slash.Command {
	return slash.Command{
		Name:        "/think",
		Aliases:     []string{"/effort"},
		Description: "Set thinking level: off, on, low, medium, high",
		Hints:       "[off|on|low|medium|high]",
		Category:    "agent",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				// Show current status
				return formatThinkStatus(c.Resolved().Think), nil
			}

			var think thinking.Value

			switch args[0] {
			case string(thinking.Low), string(thinking.Medium), string(thinking.High):
				think = thinking.NewValue(args[0])
			default:
				val, err := truthy.Parse(args[0])
				if err != nil {
					return "", fmt.Errorf("invalid value %q: %w", args[0], slash.ErrUsage)
				}

				think = thinking.NewValue(val)
			}

			if err := c.SetThink(think); err != nil {
				return "", err
			}

			return formatThinkStatus(think), nil
		},
	}
}

func formatThinkStatus(think thinking.Value) string {
	if think.IsBool() {
		if think.Bool() {
			return "Think: true"
		}

		return "Think: false"
	}

	return "Think: " + think.String()
}
