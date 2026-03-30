package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/slash"
)

// Tools creates the /tools command to list enabled tools.
func Tools() slash.Command {
	return slash.Command{
		Name:        "/tools",
		Description: "List enabled tools for the current agent/mode",
		Category:    "tools",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "debug" {
				return toolsDebug(c)
			}

			names := c.ToolNames()
			if len(names) == 0 {
				return "No tools enabled", nil
			}

			var b strings.Builder
			b.WriteString("Enabled tools:\n")

			for _, name := range names {
				b.WriteString("- " + name + "\n")
			}

			return strings.TrimRight(b.String(), "\n"), nil
		},
	}
}

func toolsDebug(c slash.Context) (string, error) {
	r := c.Resolved()

	traces, err := c.Cfg().ToolsWithTrace(r.Agent, r.Mode, c.Runtime())
	if err != nil {
		return "", err
	}

	var included, excluded []struct{ name, reason string }

	for _, t := range traces {
		entry := struct{ name, reason string }{t.Name, t.Reason}
		if t.Included {
			included = append(included, entry)
		} else {
			excluded = append(excluded, entry)
		}
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Included (%d):\n", len(included))

	for _, t := range included {
		b.WriteString("  " + t.name + "\n")
	}

	if len(excluded) > 0 {
		fmt.Fprintf(&b, "\nExcluded (%d):\n", len(excluded))

		for _, t := range excluded {
			fmt.Fprintf(&b, "  %s — %s\n", t.name, t.reason)
		}
	}

	return strings.TrimRight(b.String(), "\n"), nil
}
