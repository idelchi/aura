// Package show implements the `aura show` CLI command for listing and inspecting
// config entities loaded from disk.
package show

import (
	"context"
	"fmt"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
)

// Command creates the 'aura show' command with per-entity subcommands.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:        "show",
		Usage:       "Show config entities (agents, modes, prompts, providers, hooks, features, plugins, skills, tasks)",
		Description: "List and inspect configuration entities loaded from disk. Use subcommands to select the entity type.",
		Before: func(_ context.Context, _ *cli.Command) (context.Context, error) {
			flags := core.GetFlags()
			debug.Init(flags.WriteHome(), flags.Debug)

			return nil, nil
		},
		After: func(_ context.Context, _ *cli.Command) error {
			debug.Close()

			return nil
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			flags := core.GetFlags()

			cfg, _, err := config.New(flags.ConfigOptions(), config.AllParts()...)
			if err != nil {
				return err
			}

			fmt.Printf("agents:    %d\n", len(cfg.Agents))
			fmt.Printf("modes:     %d\n", len(cfg.Modes))
			fmt.Printf("prompts:   %d\n", len(cfg.Systems))
			fmt.Printf("providers: %d\n", len(cfg.Providers))
			fmt.Printf("hooks:     %d\n", len(cfg.Hooks))
			fmt.Printf("plugins:   %d\n", len(cfg.Plugins))
			fmt.Printf("skills:    %d\n", len(cfg.Skills))
			fmt.Printf("tasks:     %d\n", len(cfg.Tasks))

			return nil
		},
		Commands: []*cli.Command{
			agentsCommand(flags),
			modesCommand(flags),
			promptsCommand(flags),
			providersCommand(flags),
			hooksCommand(flags),
			featuresCommand(flags),
			pluginsCommand(flags),
			skillsCommand(flags),
			tasksCommand(flags),
		},
	}
}

// filterFlag returns the shared --filter flag definition.
func filterFlag(flags *core.Flags) cli.Flag {
	return &cli.StringSliceFlag{
		Name:        "filter",
		Aliases:     []string{"f"},
		Usage:       "Filter by YAML key path (key=value, repeatable, dot notation, wildcards supported)",
		Destination: &flags.ShowCmd.Filter,
	}
}

// entityCommand creates a standardized entity subcommand.
// listFn handles no-arg listing, detailFn handles single-entity detail view.
func entityCommand(
	name, usage string,
	flags *core.Flags,
	parts []config.Part,
	listFn func(*config.Config, []string) error,
	detailFn func(*config.Config, string) error,
) *cli.Command {
	return &cli.Command{
		Name:  name,
		Usage: usage,
		Flags: []cli.Flag{filterFlag(flags)},
		Action: func(_ context.Context, cmd *cli.Command) error {
			f := core.GetFlags()

			opts := f.ConfigOptions()

			opts.ExtraTaskFiles = f.Tasks.Files

			cfg, _, err := config.New(opts, parts...)
			if err != nil {
				return err
			}

			if cmd.Args().Present() {
				entityName := strings.Join(cmd.Args().Slice(), " ")

				return detailFn(&cfg, entityName)
			}

			return listFn(&cfg, f.ShowCmd.Filter)
		},
	}
}
