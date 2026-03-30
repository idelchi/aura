package slash

import (
	"context"

	"github.com/idelchi/aura/internal/config"
)

// FromCustomCommands converts loaded custom command configs into slash commands.
// Each command has Forward=true so the rendered body is sent to the LLM as a user message.
func FromCustomCommands(cmds config.Collection[config.CustomCommand]) []Command {
	var commands []Command

	for _, cmd := range cmds {
		body := cmd.Body
		meta := cmd.Metadata

		commands = append(commands, Command{
			Name:        "/" + meta.Name,
			Description: meta.Description,
			Hints:       meta.Hints,
			Category:    "custom",
			Forward:     true,
			Execute: func(_ context.Context, _ Context, args ...string) (string, error) {
				return config.Populate(body, args...), nil
			},
		})
	}

	return commands
}
