package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// Insert creates the /insert command to inject a message into history.
func Insert() slash.Command {
	return slash.Command{
		Name:        "/insert",
		Description: "Insert a message with given role into history",
		Hints:       "<system|user|assistant> <content>",
		Category:    "context",
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) < 2 {
				return "", slash.ErrUsage
			}

			var role roles.Role

			switch args[0] {
			case "system":
				role = roles.System
			case "user":
				role = roles.User
			case "assistant":
				role = roles.Assistant
			default:
				return "", fmt.Errorf("invalid role %q (use system, user, or assistant): %w", args[0], slash.ErrUsage)
			}

			content := strings.Join(args[1:], " ")
			c.Builder().InjectMessage(ctx, role, content)

			return fmt.Sprintf("Inserted %s message", args[0]), nil
		},
	}
}
