package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/slash"
)

// Skill creates the /skill command to invoke one loaded skill explicitly.
func Skill() slash.Command {
	return slash.Command{
		Name:        "/skill",
		Description: "Invoke a loaded skill",
		Hints:       "<name>",
		Category:    "tools",
		Forward:     true,
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) != 1 {
				return "", slash.ErrUsage
			}

			return skillPrompt(c.Cfg().Skills, args[0])
		},
	}
}

func skillPrompt(skills config.Collection[config.Skill], name string) (string, error) {
	configured := skills.Get(name)
	if configured == nil {
		return "", fmt.Errorf("unknown skill %q (available: %s)", name, strings.Join(skills.Names(), ", "))
	}

	return fmt.Sprintf(
		"Follow the explicitly invoked skill %q:\n\n%s",
		configured.Metadata.Name,
		configured.Instructions(),
	), nil
}
