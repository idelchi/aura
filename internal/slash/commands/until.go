package commands

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/slash"
)

// Until creates the /until command for looping until a condition is met.
func Until() slash.Command {
	return slash.Command{
		Name:        "/until",
		Description: `Loop until condition is met: /until [--max N] [not] <condition> "action1" ["action2" ...]`,
		Hints:       `[--max N] [not] <condition> "action1" ["action2" ...]`,
		Category:    "execution",
		Silent:      true,
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) < 2 {
				return "", errors.New(`usage: /until [--max N] [not] <condition> "action1" ["action2" ...]`)
			}

			i := 0
			max := 0

			// Parse optional --max N
			if i < len(args) && args[i] == "--max" {
				i++
				if i >= len(args) {
					return "", errors.New("--max requires a numeric argument")
				}

				n, err := strconv.Atoi(args[i])
				if err != nil || n <= 0 {
					return "", fmt.Errorf("--max must be a positive integer, got %q", args[i])
				}

				max = n
				i++
			}

			// Parse optional "not" prefix
			negate := false

			if i < len(args) && args[i] == "not" {
				negate = true
				i++
			}

			if i >= len(args) {
				return "", errors.New(`usage: /until [--max N] [not] <condition> "action1" ["action2" ...]`)
			}

			cond := args[i]
			i++

			actions := args[i:]
			if len(actions) == 0 {
				return "", errors.New(`usage: /until [--max N] [not] <condition> "action1" ["action2" ...]`)
			}

			for attempt := 1; max == 0 || attempt <= max; attempt++ {
				if err := ctx.Err(); err != nil {
					return "", err
				}

				met := checkCondition(cond, c)

				if negate {
					met = !met
				}

				debug.Log("[until] attempt %d: %q (negate=%v) → %v", attempt, cond, negate, met)

				if met {
					return "", nil
				}

				for _, action := range actions {
					if err := c.ProcessInput(ctx, action); err != nil {
						return "", fmt.Errorf("/until action %q: %w", action, err)
					}
				}
			}

			return "", fmt.Errorf("/until: condition %q not met after %d attempts", cond, max)
		},
	}
}
