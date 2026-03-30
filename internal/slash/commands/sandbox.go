package commands

import (
	"context"
	"errors"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/sandbox"
	"github.com/idelchi/aura/pkg/truthy"
)

// Sandbox creates the /sandbox command for controlling sandbox state.
func Sandbox() slash.Command {
	return slash.Command{
		Name:        "/sandbox",
		Aliases:     []string{"/landlock"},
		Description: "Show or toggle sandbox restrictions",
		Hints:       "[on|off]",
		Category:    "tools",
		Execute: func(_ context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) == 0 {
				if !sandbox.IsAvailable() {
					return "Landlock not available on this kernel", nil
				}

				return c.SandboxDisplay(), nil
			}

			val, err := truthy.Parse(args[0])
			if err != nil {
				return "", errors.Join(err, slash.ErrUsage)
			}

			if val && !sandbox.IsAvailable() {
				return "", errors.New("landlock not available on this kernel")
			}

			if err := c.SetSandbox(val); err != nil {
				return "", err
			}

			if val {
				return c.SandboxDisplay(), nil
			}

			return "Sandbox disabled", nil
		},
	}
}
