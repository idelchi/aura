package login

import (
	"context"
	"errors"
	"fmt"

	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/pkg/auth"
	codexAuth "github.com/idelchi/aura/pkg/auth/codex"
	copilotAuth "github.com/idelchi/aura/pkg/auth/copilot"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Command returns the `aura login` command for device code authentication.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:  "login",
		Usage: "Authenticate with an OAuth provider",
		Description: "Run a device code flow to authenticate with copilot or codex.\n\n" +
			"Examples:\n" +
			"  aura login copilot\n" +
			"  aura login --local codex",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "local",
				Usage:       "Save token to project config (--config/auth/) instead of global (~/.aura/auth/)",
				Destination: &flags.Login.Local,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return errors.New("expected 1 argument")
			}

			provider := cmd.Args().Get(0)

			var (
				token string
				err   error
			)

			switch provider {
			case "copilot":
				token, err = copilotAuth.Login(ctx)
			case "codex":
				token, err = codexAuth.Login(ctx)
			default:
				return fmt.Errorf("login not supported for provider %q (supported: copilot, codex)", provider)
			}

			if err != nil {
				return fmt.Errorf("login: %w", err)
			}

			flags := core.GetFlags()

			var dir string

			if flags.Login.Local {
				if flags.WriteHome() == "" {
					return errors.New("no config directory found — run 'aura init' first or pass --config")
				}

				dir = folder.New(flags.WriteHome(), "auth").Path()
			} else {
				if flags.Home == "" {
					return errors.New("no global home configured — use --local or set --home")
				}

				dir = folder.New(flags.Home, "auth").Path()
			}

			if err := auth.Save(dir, provider, token); err != nil {
				return fmt.Errorf("saving token: %w", err)
			}

			fmt.Fprintf(cmd.ErrWriter, "Authenticated with %s. Token saved to %s\n", provider, auth.Path(dir, provider))

			return nil
		},
	}
}
