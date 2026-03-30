package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/truthy"
)

// ReadBefore creates the /readbefore command to configure read-before enforcement.
func ReadBefore() slash.Command {
	return slash.Command{
		Name:        "/readbefore",
		Aliases:     []string{"/rb"},
		Description: "Configure read-before enforcement for file operations",
		Hints:       "[write|delete|all] [on|off]",
		Category:    "config",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			policy := c.ReadBeforePolicy()

			if len(args) == 0 {
				return fmt.Sprintf("Read-before: write=%s, delete=%s",
					onOff(policy.Write), onOff(policy.Delete)), nil
			}

			if len(args) < 2 {
				return "", errors.New("usage: /readbefore [write|delete|all] [on|off]")
			}

			val, err := truthy.Parse(args[1])
			if err != nil {
				return "", err
			}

			switch strings.ToLower(args[0]) {
			case "write":
				policy.Write = val
			case "delete":
				policy.Delete = val
			case "all":
				policy.Write = val
				policy.Delete = val
			default:
				return "", fmt.Errorf("unknown operation %q: use write, delete, or all", args[0])
			}

			if err := c.SetReadBeforePolicy(policy); err != nil {
				return "", fmt.Errorf("applying read-before policy: %w", err)
			}

			return fmt.Sprintf("Read-before: write=%s, delete=%s",
				onOff(policy.Write), onOff(policy.Delete)), nil
		},
	}
}

// onOff returns "on" or "off" for a boolean value.
func onOff(b bool) string {
	if b {
		return "on"
	}

	return "off"
}
