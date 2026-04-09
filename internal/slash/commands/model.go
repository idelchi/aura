package commands

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/model"
)

// Model creates the /model command for listing and switching models across providers.
func Model() slash.Command {
	return slash.Command{
		Name:        "/model",
		Description: "List models or switch model",
		Hints:       "[model] or [provider/model]",
		Category:    "agent",
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) > 0 {
				return switchModel(ctx, c, strings.Join(args, " "))
			}

			return listModels(ctx, c)
		},
	}
}

// switchModel parses "provider/model" or just "model" and switches the active model.
// If no provider prefix is given, the current provider is used.
func switchModel(ctx context.Context, c slash.Context, arg string) (string, error) {
	providerName, modelName, ok := strings.Cut(arg, "/")
	if !ok || c.Cfg().Providers.Get(providerName) == nil {
		// No provider prefix, or prefix isn't a known provider — treat entire arg as model name
		modelName = arg
		providerName = c.Resolved().Provider
	}

	if err := c.SwitchModel(ctx, providerName, modelName); err != nil {
		return "", err
	}

	return fmt.Sprintf("Switched to %s/%s", providerName, modelName), nil
}

// listModels queries all configured providers in parallel and opens the picker overlay.
// Results are cached per-session — subsequent calls skip the network fetch.
// Cache is cleared by /reload.
func listModels(ctx context.Context, c slash.Context) (string, error) {
	cfg := c.Cfg()
	resolved := c.Resolved()

	// Check cache first
	results := c.ModelListCache()
	if results == nil {
		// Cache miss — fetch from providers
		c.EventChan() <- ui.SpinnerMessage{Text: "Fetching models..."}

		names := c.Runtime().DisplayProviders
		if len(names) == 0 {
			names = cfg.Providers.Filter(func(_ string, p config.Provider) bool {
				return p.Can(config.CapChat)
			}).Names()
		}

		results = make([]slash.ProviderModels, len(names))

		var wg sync.WaitGroup

		for i, name := range names {
			wg.Go(func() {
				results[i] = fetchProviderModels(ctx, name, cfg)
			})
		}

		wg.Wait()

		// Sort: local providers first (alphabetical), cloud providers last
		provs := cfg.Providers

		slices.SortFunc(results, func(a, b slash.ProviderModels) int {
			ac := isCloud(a.Name, provs)
			bc := isCloud(b.Name, provs)

			if ac != bc {
				if ac {
					return 1
				}

				return -1
			}

			return strings.Compare(a.Name, b.Name)
		})

		c.CacheModelList(results)
	}

	// Build picker items from results (always rebuilt — Current depends on active model)
	var items []ui.PickerItem

	for _, r := range results {
		if r.Err != nil {
			items = append(items, ui.PickerItem{
				Group:    r.Name + " (unreachable)",
				Disabled: true,
			})

			continue
		}

		for _, m := range r.Models {
			items = append(items, ui.PickerItem{
				Group:   r.Name,
				Label:   m.Name,
				Icons:   modelIcons(m),
				Current: r.Name == resolved.Provider && m.Name == resolved.Model,
				Action:  ui.SelectModel{Provider: r.Name, Model: m.Name},
			})
		}
	}

	if len(items) == 0 {
		return "", errors.New("no providers configured")
	}

	// Send picker event to TUI
	c.EventChan() <- ui.PickerOpen{Title: "Select model:", Items: items}

	return "", nil
}

// fetchProviderModels creates a provider instance and fetches its models with a timeout.
func fetchProviderModels(ctx context.Context, name string, cfg config.Config) slash.ProviderModels {
	provCfg := cfg.Providers.Get(name)
	if provCfg == nil {
		return slash.ProviderModels{Name: name, Err: errors.New("not configured")}
	}

	provider, err := providers.New(*provCfg)
	if err != nil {
		return slash.ProviderModels{Name: name, Err: err}
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	models, err := provider.Models(ctx)
	if err != nil {
		return slash.ProviderModels{Name: name, Err: err}
	}

	// Exclude embedding-only models — they're useless in a chat interface.
	var chat model.Models

	for _, m := range models {
		if !m.Capabilities.Embedding() {
			chat = append(chat, m)
		}
	}

	filtered := provCfg.Models.Apply(chat)

	return slash.ProviderModels{Name: name, Models: filtered.ByName()}
}

// isCloud returns true if the named provider is a cloud provider (e.g. openrouter).
func isCloud(name string, provs config.StringCollection[config.Provider]) bool {
	p := provs.Get(name)

	return p != nil && p.Type == "openrouter"
}

// modelIcons returns capability icons for a model.
func modelIcons(m model.Model) string {
	var icons string

	if m.Capabilities.Thinking() {
		icons += " T"
	}

	if m.Capabilities.Vision() {
		icons += " V"
	}

	return icons
}
