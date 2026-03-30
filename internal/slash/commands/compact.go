package commands

import (
	"context"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/ui"
)

// Compact creates the /compact command to manually compact conversation context.
func Compact() slash.Command {
	return slash.Command{
		Name:        "/compact",
		Description: "Compact conversation context by summarizing old messages",
		Category:    "context",
		Execute: func(ctx context.Context, c slash.Context, _ ...string) (string, error) {
			c.EventChan() <- ui.SpinnerMessage{Text: "Compacting..."}

			err := c.Compact(ctx, true)

			c.EventChan() <- ui.SpinnerMessage{} // clear

			if err != nil {
				return "", err
			}

			return "", nil
		},
	}
}
