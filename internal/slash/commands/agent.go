package commands

import (
	"context"
	"strings"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/slash"
)

// Agent creates the /agent command to switch or list agents.
func Agent() slash.Command {
	return slash.Command{
		Name:        "/agent",
		Description: "Switch agent or list available",
		Hints:       "[name]",
		Category:    "agent",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				names := c.Cfg().Agents.Filter(config.Agent.Visible).Names()

				return "Available agents: " + strings.Join(names, ", "), nil
			}

			if err := c.SwitchAgent(args[0], "user"); err != nil {
				return "", err
			}

			return "Switched to agent: " + args[0], nil
		},
	}
}
