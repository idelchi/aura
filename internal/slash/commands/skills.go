package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/slash"
)

// Skills creates the /skills command to list loaded skills.
func Skills() slash.Command {
	return slash.Command{
		Name:        "/skills",
		Description: "List loaded skills",
		Category:    "tools",
		Execute: func(_ context.Context, c slash.Context, _ ...string) (string, error) {
			skills := c.Cfg().Skills
			names := skills.Names()

			if len(names) == 0 {
				return "No skills loaded", nil
			}

			var b strings.Builder
			b.WriteString("Loaded skills:\n")

			for _, name := range names {
				skill := skills.Get(name)
				fmt.Fprintf(&b, "- %s — %s\n", name, skill.Metadata.Description)
			}

			return strings.TrimRight(b.String(), "\n"), nil
		},
	}
}
