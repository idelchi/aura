package web

import (
	"context"

	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/ui"
	webui "github.com/idelchi/aura/internal/ui/web"
)

// Command creates the 'aura web' command for the browser-based UI.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:        "web",
		Usage:       "Start browser-based UI",
		Description: "Start an HTTP server for a browser-based chat interface with SSE streaming, htmx rendering, and session persistence. Opens the default browser automatically on startup.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "bind",
				Usage:       "Address to bind",
				Value:       "127.0.0.1:9999",
				Destination: &flags.Web.Bind,
				Sources:     cli.EnvVars("AURA_WEB_BIND"),
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			flags := core.GetFlags()
			bind := flags.Web.Bind

			webUI := func(_ core.Flags) (ui.UI, error) {
				return webui.New(bind), nil
			}

			return core.RunSession(flags, webUI, core.RunInteractive)
		},
	}
}
