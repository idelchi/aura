package commands

import (
	"context"

	"github.com/idelchi/aura/internal/slash"
)

// Exit creates the /exit command for graceful application exit.
func Exit() slash.Command {
	return slash.Command{
		Name:        "/exit",
		Aliases:     []string{"/quit"},
		Description: "Exit the application",
		Category:    "system",
		Execute: func(_ context.Context, c slash.Context, _ ...string) (string, error) {
			c.RequestExit()

			return "Exiting...", nil
		},
	}
}
