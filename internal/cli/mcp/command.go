package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/fatih/color"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	mcppkg "github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/pkg/spinner"
)

// Command creates the 'aura mcp' command for listing MCP servers and their tools.
func Command() *cli.Command {
	return &cli.Command{
		Name:        "mcp",
		Usage:       "List configured MCP servers and their tools",
		Description: "Connect to all enabled MCP servers and display their discovered tools. Disabled servers are shown but not connected. Use --include-mcps/--exclude-mcps root flags to filter which servers are loaded.",
		Before: func(_ context.Context, _ *cli.Command) (context.Context, error) {
			flags := core.GetFlags()
			debug.Init(flags.WriteHome(), flags.Debug)

			return nil, nil
		},
		After: func(_ context.Context, _ *cli.Command) error {
			debug.Close()

			return nil
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return errors.New("unexpected arguments")
			}

			ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
			defer stop()

			flags := core.GetFlags()

			cfg, _, err := config.New(flags.ConfigOptions(), config.PartMCPs, config.PartFeatures)
			if err != nil {
				return err
			}

			mcpInclude, mcpExclude := cfg.MCPFilter(nil)
			mcps := config.FilterMCPs(cfg.MCPs, mcpInclude, mcpExclude)

			names := mcps.Names()
			if len(names) == 0 {
				fmt.Fprintln(cmd.Writer, "No MCP servers configured.")

				return nil
			}

			// Collect enabled servers for parallel connection.
			enabled := make(map[string]mcppkg.Server)

			for _, name := range names {
				if s := mcps[name]; s.IsEnabled() {
					enabled[name] = s
				}
			}

			// Connect to enabled servers.
			var results []mcppkg.Result

			if len(enabled) > 0 {
				s := spinner.New("Connecting to MCP servers...")
				s.Start()

				results = mcppkg.ConnectAll(ctx, enabled, func(name string) {
					s.Update(fmt.Sprintf("Connecting to %s...", name))
				})

				s.Stop()

				// Close sessions when done.
				for _, r := range results {
					if r.Session != nil {
						defer r.Session.Close()
					}
				}
			}

			// Index results by name for lookup during display.
			resultsByName := make(map[string]mcppkg.Result, len(results))
			for _, r := range results {
				resultsByName[r.Name] = r
			}

			header := color.New(color.FgWhite, color.Bold, color.Underline).SprintFunc()
			errColor := color.New(color.FgRed).SprintFunc()

			for i, name := range names {
				server := mcps[name]

				if i > 0 {
					fmt.Fprintln(cmd.Writer)
				}

				// Header: NAME (type) [disabled]
				label := header(strings.ToUpper(name)) + " (" + server.EffectiveType() + ")"
				if server.Disabled {
					label += " " + errColor("[disabled]")
				}

				fmt.Fprintln(cmd.Writer, label)
				fmt.Fprintln(cmd.Writer, "  "+server.Display())

				// For disabled servers, just show config — no connection.
				if server.Disabled {
					continue
				}

				r, ok := resultsByName[name]
				if !ok {
					continue
				}

				if r.Error != nil {
					fmt.Fprintln(cmd.Writer, "  "+errColor("error: "+r.Error.Error()))

					continue
				}

				toolNames := r.ToolNames()
				if len(toolNames) == 0 {
					fmt.Fprintln(cmd.Writer, "  tools: (none)")

					continue
				}

				fmt.Fprintln(cmd.Writer, "  tools:")

				for _, t := range toolNames {
					fmt.Fprintln(cmd.Writer, "    - "+t)
				}
			}

			return nil
		},
	}
}
