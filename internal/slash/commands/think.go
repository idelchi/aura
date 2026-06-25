package commands

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// Think creates the /think command to set thinking mode.
func Think() slash.Command {
	return slash.Command{
		Name:        "/think",
		Aliases:     []string{"/effort"},
		Description: "Set thinking: off, on/auto, none, minimal, low, medium, high, xhigh, max",
		Hints:       "[off|on|auto|none|minimal|low|medium|high|xhigh|max]",
		Category:    "agent",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				// Show current status
				return formatThinkStatus(c.Resolved().Think), nil
			}

			think, err := thinking.ParseValue(args[0])
			if err != nil {
				return "", fmt.Errorf("%w: %w", err, slash.ErrUsage)
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
			return "Think: auto"
		}

		return "Think: off"
	}

	return "Think: " + think.String()
}
