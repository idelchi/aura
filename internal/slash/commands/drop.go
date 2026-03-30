package commands

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/idelchi/aura/internal/slash"
)

// Drop creates the /drop command to remove messages from history.
func Drop() slash.Command {
	return slash.Command{
		Name:        "/drop",
		Aliases:     []string{"/remove"},
		Description: "Drop last N messages from history",
		Hints:       "[n|all]",
		Category:    "context",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			length := c.Builder().Len()
			if length <= 1 {
				return "No messages to drop", nil
			}

			// Default: drop 1
			if len(args) == 0 {
				c.Builder().DropN(1)

				return "Dropped last message", nil
			}

			arg := args[0]

			// Handle "all"
			if arg == "all" {
				c.Builder().DropN(length)

				return "Dropped all messages", nil
			}

			// Parse number
			n, err := strconv.Atoi(arg)
			if err != nil {
				return "", errors.Join(err, slash.ErrUsage)
			}

			if n < 1 {
				return "", errors.New("number must be at least 1")
			}

			// Cap at available messages (minus system prompt)
			available := length - 1
			if n > available {
				n = available
			}

			c.Builder().DropN(n)

			return fmt.Sprintf("Dropped %d message(s)", n), nil
		},
	}
}
