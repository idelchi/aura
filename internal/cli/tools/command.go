package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/fatih/color"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/tools/assemble"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/sandbox"
	"github.com/idelchi/aura/sdk"
)

// Command creates the 'aura tools' command for listing, inspecting, or executing tools.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:      "tools",
		ArgsUsage: "[name] [args...]",
		Usage:     "List, inspect, or execute tools",
		Description: heredoc.Doc(`
			List available tools, show tool details, or execute a tool directly.

			Arguments can be passed as JSON or as key=value pairs:
			  aura tools Read '{"path": "main.go", "line_start": 1}'
			  aura tools Read path=main.go line_start=1

			Key-value pairs are auto-detected. Use --raw to force JSON parsing.
		`),
		Before: func(_ context.Context, _ *cli.Command) (context.Context, error) {
			flags := core.GetFlags()
			debug.Init(flags.WriteHome(), flags.Debug)

			return nil, nil
		},
		After: func(_ context.Context, _ *cli.Command) error {
			debug.Close()

			return nil
		},
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Category:    "Output:",
				Usage:       "Output as structured JSON",
				Destination: &flags.Tools.JSON,
			},
			&cli.BoolFlag{
				Name:        "raw",
				Category:    "Output:",
				Usage:       "Force JSON parsing of arguments",
				Destination: &flags.Tools.Raw,
			},
			&cli.BoolFlag{
				Name:        "headless",
				Category:    "Output:",
				Aliases:     []string{"H"},
				Usage:       "No printouts during sandbox setup",
				Destination: &flags.Tools.Headless,
			},
			&cli.BoolFlag{
				Name:        "ro",
				Category:    "Sandbox:",
				Aliases:     []string{"R"},
				Usage:       "Apply read-only sandboxing",
				Destination: &flags.Tools.RO,
			},
			&cli.StringSliceFlag{
				Name:        "ro-paths",
				Category:    "Sandbox:",
				Aliases:     []string{"O"},
				Usage:       "Additional read-only paths for sandboxing",
				Destination: &flags.Tools.ROPaths,
			},
			&cli.StringSliceFlag{
				Name:        "rw-paths",
				Category:    "Sandbox:",
				Aliases:     []string{"W"},
				Usage:       "Additional read-write paths for sandboxing",
				Destination: &flags.Tools.RWPaths,
			},
			&cli.BoolFlag{
				Name:        "with-mcp",
				Category:    "MCP:",
				Usage:       "Connect to MCP servers and include their tools",
				Destination: &flags.Tools.WithMCP,
			},
			&cli.StringSliceFlag{
				Name:        "mcp-servers",
				Category:    "MCP:",
				Usage:       "Only connect to named MCP servers (implies --with-mcp)",
				Destination: &flags.Tools.MCPServers,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := core.GetFlags()
			tf := flags.Tools

			opts := flags.ConfigOptions()

			opts.WithPlugins = !flags.WithoutPlugins
			opts.UnsafePlugins = flags.UnsafePlugins

			cfg, paths, err := config.New(opts,
				config.PartFeatures, config.PartToolDefs, config.PartLSP, config.PartAgents,
				config.PartModes, config.PartSystems, config.PartAgentsMd,
				config.PartSkills, config.PartPlugins, config.PartProviders, config.PartMCPs,
			)
			if err != nil {
				return core.OutputSetupError(cmd.Writer, tf.JSON, err)
			}

			// Load plugins for tool registration.
			var pluginCache *plugins.Cache

			if opts.WithPlugins {
				pluginCache, err = plugins.LoadAll(cfg.Plugins, cfg.Features.PluginConfig, paths.Home)
				if err != nil {
					return core.OutputSetupError(cmd.Writer, tf.JSON, fmt.Errorf("loading plugins: %w", err))
				}

				defer pluginCache.Close()
			}

			// Wire events channel for tool progress when interactive.
			var (
				events   chan<- ui.Event
				eventsCh chan ui.Event
			)

			interactive := !tf.Headless

			if interactive {
				eventsCh = make(chan ui.Event, 100)
				events = eventsCh
			}

			// Create LSP manager if configured.
			var lspManager *lsp.Manager

			if len(cfg.LSPServers) > 0 {
				lspManager = lsp.NewManager(lsp.Config{Servers: map[string]lsp.Server(cfg.LSPServers)}, core.WorkDir)

				defer lspManager.StopAll()
			}

			estimator, err := cfg.Features.Estimation.NewEstimator()
			if err != nil {
				return core.OutputSetupError(cmd.Writer, tf.JSON, fmt.Errorf("creating estimator: %w", err))
			}

			rt := &config.Runtime{Estimator: estimator}

			result, err := assemble.Tools(assemble.Params{
				Config:      cfg,
				Paths:       paths,
				Runtime:     rt,
				TodoList:    todo.New(),
				Events:      events,
				PluginCache: pluginCache,
				LSPManager:  lspManager,
				Estimate:    estimator.EstimateLocal,
				ForDisplay:  true,
			})
			if err != nil {
				return core.OutputSetupError(cmd.Writer, tf.JSON, err)
			}

			allTools := result.Tools

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			ctx = tool.WithWorkDir(ctx, core.WorkDir)

			withMCP := tf.WithMCP || len(tf.MCPServers) > 0

			if withMCP {
				mcpInclude, mcpExclude := cfg.MCPFilter(nil)

				sessions, warnings := core.ConnectMCPServers(
					ctx,
					config.FilterMCPs(cfg.MCPs, mcpInclude, mcpExclude),
					tf.MCPServers,
					interactive,
				)
				for _, s := range sessions {
					defer s.Close()

					for _, t := range s.Tools() {
						allTools.Add(t)
					}
				}

				for _, w := range warnings {
					fmt.Fprintln(cmd.ErrWriter, w)
				}
			}

			debug.Log("tools loaded: %d total", len(allTools))

			args := cmd.Args().Slice()

			switch len(args) {
			case 0:
				// List all tools
				for _, t := range allTools {
					fmt.Fprintln(cmd.Writer, "- "+t.Name())
				}
			case 1:
				// Show tool details
				t, err := allTools.Get(args[0])
				if err != nil {
					return core.OutputError(
						cmd.Writer, tf.JSON,
						fmt.Errorf("tool %q not found\nAvailable: %v", args[0], allTools.Names()),
					)
				}

				printToolDetails(cmd, t)
			default:
				// Execute tool (2+ args: tool name + arguments)
				t, err := allTools.Get(args[0])
				if err != nil {
					return core.OutputError(
						cmd.Writer, tf.JSON,
						fmt.Errorf("tool %q not found\nAvailable: %v", args[0], allTools.Names()),
					)
				}

				readOnlyPaths := tf.ROPaths
				readWritePaths := tf.RWPaths

				if tf.RO {
					readOnlyPaths = []string{"/"}
					readWritePaths = []string{}
				}

				// Apply sandbox right before execution
				if err := applySandbox(cmd.Writer, readOnlyPaths, readWritePaths, tf.Headless); err != nil {
					return core.OutputError(cmd.Writer, tf.JSON, err)
				}

				argsMap, err := parseArgs(args[1:], t.Schema(), tf.Raw)
				if err != nil {
					return core.OutputError(cmd.Writer, tf.JSON, err)
				}

				// Read sdk.Context from stdin (sandbox re-exec pipes it from parent).
				// Gate on pipe check — io.ReadAll on a terminal fd blocks forever.
				if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
					if stdinData, err := io.ReadAll(os.Stdin); err == nil && len(stdinData) > 0 {
						var sdkCtx sdk.Context
						if err := json.Unmarshal(stdinData, &sdkCtx); err != nil {
							debug.Log("sandbox context unmarshal: %v", err)
						} else {
							ctx = tool.WithSDKContext(ctx, sdkCtx)
						}
					}
				}

				output, execErr := core.ExecuteTool(ctx, t, argsMap, eventsCh, interactive)

				return core.OutputResult(cmd.Writer, tf.JSON, output, execErr)
			}

			return nil
		},
	}
}

// applySandbox applies Landlock sandbox with the given paths.
func applySandbox(w io.Writer, readOnlyPaths, readWritePaths []string, headless bool) error {
	if !sandbox.IsAvailable() {
		return nil
	}

	if len(readOnlyPaths) == 0 && len(readWritePaths) == 0 {
		return nil
	}

	sb := &sandbox.Sandbox{WorkDir: core.WorkDir}

	sb.AddReadOnly(readOnlyPaths...)
	sb.AddReadWrite(readWritePaths...)

	if err := sb.Apply(); err != nil {
		return fmt.Errorf("applying sandbox: %w", err)
	}

	if !headless {
		fmt.Fprintln(w, color.RedString(sb.String()))
	}

	return nil
}

// printToolDetails displays detailed information about a tool.
func printToolDetails(cmd *cli.Command, t tool.Tool) {
	fmt.Fprintln(cmd.Writer, "Name:", t.Name())
	fmt.Fprintln(cmd.Writer)
	fmt.Fprintln(cmd.Writer, "Description:")
	fmt.Fprintln(cmd.Writer, t.Description())
	fmt.Fprintln(cmd.Writer)
	fmt.Fprintln(cmd.Writer, "Usage:")
	fmt.Fprintln(cmd.Writer, t.Usage())

	if strings.TrimSpace(t.Examples()) != "" {
		fmt.Fprintln(cmd.Writer)
		fmt.Fprintln(cmd.Writer, "Examples:")
		fmt.Fprintln(cmd.Writer, strings.TrimSpace(t.Examples()))
	}

	fmt.Fprintln(cmd.Writer)
	fmt.Fprintln(cmd.Writer, "Parameters:")

	schema := t.Schema()
	for name, prop := range schema.Parameters.Properties {
		fmt.Fprintf(cmd.Writer, "  - %s: %s\n", name, prop.Description)
	}
}
