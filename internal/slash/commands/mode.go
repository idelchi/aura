package commands

import (
	"context"
	"strings"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/slash"
)

// Mode creates the /mode command to switch or list modes.
func Mode() slash.Command {
	return slash.Command{
		Name:        "/mode",
		Description: "Switch mode, clear with 'none', or list available",
		Hints:       "[name|none]",
		Category:    "agent",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				names := c.Cfg().Modes.Filter(config.Mode.Visible).Names()

				return "Available modes: " + strings.Join(names, ", "), nil
			}

			name := args[0]

			if name == "none" || name == "" {
				if err := c.SwitchMode(""); err != nil {
					return "", err
				}

				return "Mode cleared", nil
			}

			if err := c.SwitchMode(name); err != nil {
				return "", err
			}

			return "Switched to mode: " + name, nil
		},
	}
}
