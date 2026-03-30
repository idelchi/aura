package commands

import (
	"context"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/ui"
)

// Todo creates the /todo command to show or edit the current todo list.
func Todo() slash.Command {
	return slash.Command{
		Name:        "/todo",
		Description: "Show or edit the todo list",
		Hints:       "[edit]",
		Category:    "todo",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "edit" {
				if c.TodoList().IsEmpty() {
					return "No todos to edit.", nil
				}

				c.EventChan() <- ui.TodoEditRequested{Content: c.TodoList().String()}

				return "", nil
			}

			if c.TodoList().IsEmpty() {
				return "No todos.", nil
			}

			return c.TodoList().String(), nil
		},
	}
}
