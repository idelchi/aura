package commands

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/tmpl"
	"github.com/idelchi/aura/internal/ui"
)

// Replay creates the /replay command to execute commands from a YAML file.
func Replay() slash.Command {
	return slash.Command{
		Name:        "/replay",
		Description: "Replay commands from a YAML file",
		Hints:       "<file> [start-index]",
		Category:    "execution",
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				return "", slash.ErrUsage
			}

			path := args[0]
			startIndex := 0

			if len(args) > 1 {
				idx, err := strconv.Atoi(args[1])
				if err != nil {
					return "", fmt.Errorf("invalid start index %q: %w", args[1], slash.ErrUsage)
				}

				startIndex = idx
			}

			items, err := tmpl.Load(path, c.TemplateVars())
			if err != nil {
				return "", err
			}

			if len(items) == 0 {
				return "", errors.New("replay file is empty")
			}

			if startIndex < 0 || startIndex >= len(items) {
				return "", fmt.Errorf(
					"start index %d out of range (file has %d items, valid range: 0-%d)",
					startIndex,
					len(items),
					len(items)-1,
				)
			}

			executed := 0

			for i := startIndex; i < len(items); i++ {
				item := strings.TrimSpace(items[i])
				if item == "" {
					continue
				}

				// Prevent recursive replay
				if strings.HasPrefix(item, "/replay") {
					continue
				}

				if err := c.ProcessInput(ctx, item); err != nil {
					// Log error but continue
					c.EventChan() <- ui.CommandResult{Error: fmt.Errorf("replay [%d]: %w", i, err)}
				}

				// Wait for output to be consumed before next item
				done := make(chan struct{})
				c.EventChan() <- ui.Flush{Done: done}

				<-done

				executed++
			}

			return fmt.Sprintf("Replayed %d items", executed), nil
		},
	}
}
