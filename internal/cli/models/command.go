package models

import (
	"context"
	"encoding/json"
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
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/pkg/cache"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
	"github.com/idelchi/aura/pkg/providers/registry"
)

// Command creates the 'aura models' command for listing available models.
func Command(flags *core.Flags) *cli.Command {
	return &cli.Command{
		Name:        "models",
		Usage:       "List available models",
		Description: "List models from all configured providers. Use --providers to filter by provider. Models show name, context length, parameter count, and capabilities.",
		Before: func(_ context.Context, _ *cli.Command) (context.Context, error) {
			flags := core.GetFlags()
			debug.Init(flags.WriteHome(), flags.Debug)

			return nil, nil
		},
		After: func(_ context.Context, _ *cli.Command) error {
			debug.Close()

			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "sort-by",
				Usage:       "Sort models by: name, context, size",
				Value:       "size",
				Destination: &flags.Models.SortBy,
				Sources:     cli.EnvVars("AURA_MODELS_SORT_BY"),
			},
			&cli.StringSliceFlag{
				Name:        "filter",
				Aliases:     []string{"f"},
				Usage:       fmt.Sprintf("Filter by capability (%s)", strings.Join(capabilities.FilterNames(), ", ")),
				Destination: &flags.Models.Filter,
				Sources:     cli.EnvVars("AURA_MODELS_FILTER"),
			},
			&cli.StringSliceFlag{
				Name:        "name",
				Aliases:     []string{"n"},
				Usage:       "Filter by model name (wildcard patterns, repeatable)",
				Destination: &flags.Models.Name,
				Sources:     cli.EnvVars("AURA_MODELS_NAME"),
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return errors.New("unexpected arguments")
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			flags := core.GetFlags()

			cfg, _, err := config.New(flags.ConfigOptions(), config.PartProviders)
			if err != nil {
				return err
			}

			appCache := cache.New(flags.WriteHome(), flags.NoCache)
			modelsCache := appCache.Domain("models")

			// Refresh catwalk registry (needed for enrichment on cache miss).
			registry.Refresh(ctx, appCache.Domain("catwalk"))

			sortBy := flags.Models.SortBy

			// Parse capability filters once (fail fast on invalid values).
			var capFilters []capabilities.Capability

			for _, name := range flags.Models.Filter {
				cap, err := capabilities.Parse(name)
				if err != nil {
					return err
				}

				capFilters = append(capFilters, cap)
			}

			providerNames := flags.Providers
			if len(providerNames) == 0 {
				providerNames = cfg.Providers.Filter(func(_ string, p config.Provider) bool {
					return p.Can(config.CapChat)
				}).Names()
			}

			providerHeader := color.New(color.FgWhite, color.Bold, color.Underline).SprintFunc()

			for i, name := range providerNames {
				p := cfg.Providers.Get(name)
				if p == nil {
					fmt.Fprintf(cmd.ErrWriter, "provider %q not found, skipping\n", name)

					continue
				}

				allModels, fetchErr := fetchModels(ctx, name, *p, modelsCache)

				if i > 0 {
					fmt.Fprintln(cmd.Writer)
				}

				fmt.Fprintf(cmd.Writer, "%s (%s)\n", providerHeader(strings.ToUpper(name)), p.Type)

				if fetchErr != nil {
					errText := color.New(color.FgRed).Sprintf("  Error: %s", fetchErr)
					fmt.Fprintln(cmd.Writer, errText)

					continue
				}

				filtered := p.Models.Apply(allModels)

				if len(flags.Models.Name) > 0 {
					filtered = filtered.Include(flags.Models.Name...)
				}

				for _, cap := range capFilters {
					filtered = filtered.WithCapability(cap)
				}

				filtered.Display(cmd.Writer, model.SortBy(sortBy))
			}

			return nil
		},
	}
}

// fetchModels returns models for a provider, using the cache when available.
// On cache miss, fetches live from the provider and caches the result.
// Returns the error so callers can display it instead of silently showing "No models found."
func fetchModels(ctx context.Context, name string, p config.Provider, domain *cache.Domain) (model.Models, error) {
	cacheKey := name + ".json"

	if data, ok := domain.Read(cacheKey); ok {
		var cached model.Models
		if err := json.Unmarshal(data, &cached); err == nil {
			return cached, nil
		}
	}

	provider, err := providers.New(p)
	if err != nil {
		return nil, err
	}

	allModels, err := provider.Models(ctx)
	if err != nil {
		return nil, err
	}

	if data, err := json.Marshal(allModels); err == nil {
		if err := domain.Write(cacheKey, data); err != nil {
			debug.Log("[models] cache write %q: %v", name, err)
		}
	}

	return allModels, nil
}
