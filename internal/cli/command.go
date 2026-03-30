package cli

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/fatih/color"
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/mirror"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// SetEmbeddedConfig sets the embedded configuration filesystem.
func SetEmbeddedConfig(fs embed.FS) {
	core.EmbeddedConfig = fs
}

func Execute(version string) error {
	app, flags := buildApp(version)

	err := app.Run(context.Background(), os.Args)

	if flags.Mirror != nil {
		flags.Mirror.Close()

		// Restore color outputs to match restored os.Stdout/os.Stderr.
		color.Output = os.Stdout
		color.Error = os.Stderr
	}

	return err
}

func buildApp(version string) (*cli.Command, *core.Flags) {
	flags := &core.Flags{
		IsSet: core.NotSet,
		Tasks: core.TasksFlags{
			Run: core.TasksRunFlags{IsSet: core.NotSet},
		},
	}

	var configDefault []string

	if d := core.ProjectConfigDir(); d != "" {
		configDefault = []string{d}
	}

	defaultHome := ""

	if home, err := folder.Home(); err == nil {
		d := home.Join(".aura")
		if d.Exists() {
			defaultHome = d.Path()
		}
	}

	app := &cli.Command{
		Name:            "aura",
		Usage:           "Agentic coding CLI assistant",
		Description:     "Aura is a profile-based agentic coding assistant with configurable agents, tools, and plugins.",
		Version:         version,
		HideHelpCommand: true,
		Before: func(_ context.Context, cmd *cli.Command) (context.Context, error) {
			// Wire IsSet so callers can check flag presence.
			flags.IsSet = cmd.IsSet

			wd, err := folder.Cwd()
			if err != nil {
				return nil, fmt.Errorf("getting current directory: %w", err)
			}

			cwd := wd.Path()

			core.LaunchDir = cwd
			core.WorkDir = cwd

			// Output mirror — set up before any other output.
			if flags.Output != "" {
				outputFile := file.New(flags.Output)
				if !outputFile.IsAbs() {
					outputFile = file.New(cwd, flags.Output)
				}

				m, err := mirror.New(outputFile.Path())
				if err != nil {
					return nil, fmt.Errorf("output mirror: %w", err)
				}

				flags.Mirror = m

				// Reset color package outputs — they captured os.Stdout/os.Stderr
				// at init time, before the mirror replaced them.
				color.Output = os.Stdout
				color.Error = os.Stderr
			}

			// Wire Writer AFTER mirror setup so it picks up the post-mirror os.Stdout.
			// Without --output, os.Stdout is the terminal. With --output, it's the mirror pipe.
			// Tests can override flags.Writer directly with a buffer.
			flags.Writer = os.Stdout

			// urfave/cli sets cmd.Writer during setupDefaults() which runs BEFORE
			// Before hooks — so subcommand Writers capture pre-mirror os.Stdout.
			// Update the entire command tree to use post-mirror writers.
			var fixWriters func(*cli.Command)

			fixWriters = func(c *cli.Command) {
				c.Writer = os.Stdout

				c.ErrWriter = os.Stderr
				for _, sub := range c.Commands {
					fixWriters(sub)
				}
			}
			fixWriters(cmd)

			// Absolutize path flags.
			for i, cfg := range flags.Config {
				f := file.New(cfg)
				if f.Set() && !f.IsAbs() {
					flags.Config[i] = file.New(cwd, cfg).Path()
				}
			}

			envFileExplicit := flags.IsSet("env-file")

			for i, ef := range flags.EnvFile {
				f := file.New(ef)
				if f.Set() && !f.IsAbs() {
					flags.EnvFile[i] = file.New(cwd, ef).Path()
				}
			}

			if err := flags.LoadEnvFiles(envFileExplicit); err != nil {
				return nil, err
			}

			// Store flags for GetFlags() before any workdir change.
			core.StoreFlags(*flags)

			if flags.Workdir == "" {
				return nil, nil
			}

			workDir := folder.New(flags.Workdir)
			if !workDir.IsAbs() {
				workDir = folder.New(cwd, flags.Workdir)
			}

			if !workDir.Exists() {
				return nil, fmt.Errorf("workdir %q: not a directory or does not exist", workDir)
			}

			core.WorkDir = workDir.Path()

			if err := os.Chdir(workDir.Path()); err != nil {
				return nil, fmt.Errorf("chdir to workdir %q: %w", workDir, err)
			}

			return nil, nil
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unknown command %q\n\nRun 'aura --help' for available commands", cmd.Args().First())
			}

			return core.Run()
		},
		Flags: []cli.Flag{
			// Agent
			&cli.StringFlag{
				Name:        "agent",
				Local:       true,
				Category:    "Agent:",
				Usage:       "The agent to use",
				Sources:     cli.EnvVars("AURA_AGENT"),
				Destination: &flags.Agent,
			},
			&cli.StringFlag{
				Name:        "provider",
				Local:       true,
				Category:    "Agent:",
				Usage:       "LLM provider (overrides agent config)",
				Sources:     cli.EnvVars("AURA_PROVIDER"),
				Destination: &flags.Provider,
			},
			&cli.StringFlag{
				Name:        "model",
				Local:       true,
				Category:    "Agent:",
				Aliases:     []string{"m"},
				Usage:       "Model name (overrides agent config)",
				Sources:     cli.EnvVars("AURA_MODEL"),
				Destination: &flags.Model,
			},
			&cli.StringFlag{
				Name:        "mode",
				Local:       true,
				Category:    "Agent:",
				Usage:       "Starting mode (overrides agent config)",
				Sources:     cli.EnvVars("AURA_MODE"),
				Destination: &flags.Mode,
			},
			&cli.StringFlag{
				Name:        "system",
				Local:       true,
				Category:    "Agent:",
				Usage:       "System prompt (overrides agent config)",
				Sources:     cli.EnvVars("AURA_SYSTEM"),
				Destination: &flags.System,
			},
			&cli.StringFlag{
				Name:        "think",
				Local:       true,
				Category:    "Agent:",
				Usage:       "Thinking level (off, on, low, medium, high)",
				Sources:     cli.EnvVars("AURA_THINK"),
				Destination: &flags.Think,
			},

			// Filtering
			&cli.StringSliceFlag{
				Name:        "include-tools",
				Local:       true,
				Category:    "Filtering:",
				Usage:       `Glob patterns for tools to include (e.g. "Read,Glob,Rg")`,
				Sources:     cli.EnvVars("AURA_INCLUDE_TOOLS"),
				Destination: &flags.IncludeTools,
			},
			&cli.StringSliceFlag{
				Name:        "exclude-tools",
				Local:       true,
				Category:    "Filtering:",
				Usage:       `Glob patterns for tools to exclude (e.g. "Bash,Patch")`,
				Sources:     cli.EnvVars("AURA_EXCLUDE_TOOLS"),
				Destination: &flags.ExcludeTools,
			},
			&cli.StringSliceFlag{
				Name:        "include-mcps",
				Local:       true,
				Category:    "Filtering:",
				Usage:       `Glob patterns for MCP servers to connect (e.g. "context7,git*")`,
				Sources:     cli.EnvVars("AURA_INCLUDE_MCPS"),
				Destination: &flags.IncludeMCPs,
			},
			&cli.StringSliceFlag{
				Name:        "exclude-mcps",
				Local:       true,
				Category:    "Filtering:",
				Usage:       `Glob patterns for MCP servers to skip (e.g. "portainer")`,
				Sources:     cli.EnvVars("AURA_EXCLUDE_MCPS"),
				Destination: &flags.ExcludeMCPs,
			},
			&cli.StringSliceFlag{
				Name:        "providers",
				Local:       true,
				Category:    "Filtering:",
				Usage:       "Limit to these providers (filters model listings and agents)",
				Sources:     cli.EnvVars("AURA_PROVIDERS"),
				Destination: &flags.Providers,
			},

			// Limits
			&cli.IntFlag{
				Name:        "max-steps",
				Local:       true,
				Category:    "Limits:",
				Usage:       "Maximum tool-use iterations",
				Sources:     cli.EnvVars("AURA_MAX_STEPS"),
				Destination: &flags.MaxSteps,
			},
			&cli.IntFlag{
				Name:        "token-budget",
				Local:       true,
				Category:    "Limits:",
				Usage:       "Cumulative token limit",
				Sources:     cli.EnvVars("AURA_TOKEN_BUDGET"),
				Destination: &flags.TokenBudget,
			},

			// Session
			&cli.StringFlag{
				Name:        "resume",
				Local:       true,
				Category:    "Session:",
				Usage:       "Resume a saved session by ID prefix",
				Sources:     cli.EnvVars("AURA_RESUME"),
				Destination: &flags.Resume,
			},
			&cli.BoolFlag{
				Name:        "continue",
				Local:       true,
				Category:    "Session:",
				Aliases:     []string{"c"},
				Usage:       "Resume the most recent session",
				Sources:     cli.EnvVars("AURA_CONTINUE"),
				Destination: &flags.Continue,
			},

			// Config
			&cli.StringFlag{
				Name:        "home",
				Local:       true,
				Category:    "Config:",
				Value:       defaultHome,
				Usage:       "Global config home (~/.aura); set to empty to disable",
				Sources:     cli.EnvVars("AURA_HOME"),
				Destination: &flags.Home,
			},
			&cli.StringSliceFlag{
				Name:        "config",
				Local:       true,
				Category:    "Config:",
				Value:       configDefault,
				Usage:       "Configuration directories to merge (left-to-right, last wins)",
				Sources:     cli.EnvVars("AURA_CONFIG"),
				Destination: &flags.Config,
			},
			&cli.StringSliceFlag{
				Name:        "env-file",
				Local:       true,
				Category:    "Config:",
				Aliases:     []string{"e"},
				Value:       []string{"secrets.env"},
				Usage:       "Environment files to load",
				Sources:     cli.EnvVars("AURA_ENV_FILE"),
				Destination: &flags.EnvFile,
			},
			&cli.StringFlag{
				Name:        "workdir",
				Local:       true,
				Category:    "Config:",
				Aliases:     []string{"w"},
				Usage:       "Working directory for tool execution and path resolution",
				Sources:     cli.EnvVars("AURA_WORKDIR"),
				Destination: &flags.Workdir,
			},
			&cli.StringMapFlag{
				Name:        "set",
				Local:       true,
				Category:    "Config:",
				Usage:       "Set template variables (KEY=value, repeatable)",
				Sources:     cli.EnvVars("AURA_SET"),
				Destination: &flags.Set,
			},
			&cli.StringSliceFlag{
				Name:        "override",
				Local:       true,
				Category:    "Config:",
				Aliases:     []string{"O"},
				Usage:       "Override config values (dot-notation, repeatable). Example: features.tools.max_steps=10",
				Sources:     cli.EnvVars("AURA_OVERRIDE"),
				Destination: &flags.Override,
			},

			// Output
			&cli.StringFlag{
				Name:        "output",
				Local:       true,
				Category:    "Output:",
				Aliases:     []string{"o"},
				Usage:       "Mirror all output to file",
				Sources:     cli.EnvVars("AURA_OUTPUT"),
				Destination: &flags.Output,
			},
			&cli.BoolFlag{
				Name:        "show",
				Local:       true,
				Category:    "Output:",
				Aliases:     []string{"s"},
				Usage:       "Show the configuration and exit",
				Sources:     cli.EnvVars("AURA_SHOW"),
				Destination: &flags.Show,
			},
			&cli.BoolFlag{
				Name:        "print",
				Local:       true,
				Category:    "Output:",
				Usage:       "Print loaded config summary and exit",
				Sources:     cli.EnvVars("AURA_PRINT"),
				Destination: &flags.Print,
			},
			&cli.BoolFlag{
				Name:        "print-env",
				Local:       true,
				Category:    "Output:",
				Usage:       "Print resolved settings as AURA_* environment variables and exit",
				Sources:     cli.EnvVars("AURA_PRINT_ENV"),
				Destination: &flags.PrintEnv,
			},
			&cli.StringFlag{
				Name:        "dry",
				Local:       true,
				Category:    "Output:",
				Usage:       `Dry-run mode: "render" (print and exit) or "noop" (run with noop provider)`,
				Sources:     cli.EnvVars("AURA_DRY"),
				Destination: &flags.Dry,
			},
			&cli.BoolFlag{
				Name:        "debug",
				Local:       true,
				Category:    "Output:",
				Usage:       "Enable debug logging",
				Sources:     cli.EnvVars("AURA_DEBUG"),
				Destination: &flags.Debug,
			},
			&cli.BoolFlag{
				Name:        "no-cache",
				Local:       true,
				Category:    "Config:",
				Usage:       "Bypass cached data (re-fetch from source)",
				Sources:     cli.EnvVars("AURA_NO_CACHE"),
				Destination: &flags.NoCache,
			},

			// UI
			&cli.BoolFlag{
				Name:        "simple",
				Local:       true,
				Category:    "UI:",
				Usage:       "Use simple readline-based TUI",
				Sources:     cli.EnvVars("AURA_SIMPLE"),
				Destination: &flags.Simple,
			},
			&cli.BoolFlag{
				Name:        "auto",
				Local:       true,
				Category:    "UI:",
				Usage:       "Enable auto mode (continue while todo items are pending)",
				Sources:     cli.EnvVars("AURA_AUTO"),
				Destination: &flags.Auto,
			},

			// Plugins
			&cli.BoolFlag{
				Name:        "without-plugins",
				Local:       true,
				Category:    "Plugins:",
				Usage:       "Disable Go plugins",
				Sources:     cli.EnvVars("AURA_WITHOUT_PLUGINS"),
				Destination: &flags.WithoutPlugins,
			},
			&cli.BoolFlag{
				Name:        "unsafe-plugins",
				Local:       true,
				Category:    "Plugins:",
				Usage:       "Allow plugins to use os/exec and other restricted imports",
				Sources:     cli.EnvVars("AURA_UNSAFE_PLUGINS"),
				Destination: &flags.UnsafePlugins,
			},
			&cli.BoolFlag{
				Name:        "experiments",
				Local:       true,
				Category:    "Plugins:",
				Usage:       "Enable experimental features",
				Sources:     cli.EnvVars("AURA_EXPERIMENTS"),
				Destination: &flags.Experiments,
			},
		},
		Commands: buildSubcommands(flags),
	}

	return app, flags
}
