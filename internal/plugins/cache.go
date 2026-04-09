package plugins

import (
	"fmt"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/config/merge"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/pkg/wildcard"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Cache holds all loaded plugins for a session. It owns a shared temp GOPATH
// used by all plugin interpreters (following Traefik's Manager pattern).
type Cache struct {
	plugins []*Plugin
	goPath  string
}

// LoadAll creates a Cache by loading all enabled plugins from config.
// Returns a nil Cache if no plugins are configured or all are disabled.
// The features parameter controls unsafe mode and include/exclude filtering.
// configDir is the project .aura/ directory, passed to plugin Init() as ToolConfig.ConfigDir.
func LoadAll(
	cfgPlugins config.StringCollection[config.Plugin],
	features config.PluginConfig,
	configDir string,
) (*Cache, error) {
	if len(cfgPlugins) == 0 {
		return nil, nil
	}

	goDir, err := folder.CreateRandomInDir("", "aura-plugins-*")
	if err != nil {
		return nil, fmt.Errorf("creating plugin GOPATH: %w", err)
	}

	goPath := goDir.Path()

	if err := goDir.Join("src").Create(); err != nil {
		goDir.Remove()

		return nil, fmt.Errorf("creating plugin GOPATH src: %w", err)
	}

	var loaded []*Plugin

	for name, cfg := range cfgPlugins {
		if !cfg.IsEnabled() {
			debug.Log("[plugin] %q: disabled, skipping", name)

			continue
		}

		if !matchesFilters(name, features.Include, features.Exclude) {
			debug.Log("[plugin] %q: excluded by filter, skipping", name)

			continue
		}

		merged, err := mergePluginConfig(cfg.Config, features.Config.Global, features.Config.Local[name])
		if err != nil {
			goDir.Remove()

			return nil, fmt.Errorf("merging config for plugin %q: %w", name, err)
		}

		p, err := Load(name, cfg, goPath, features.Unsafe, configDir, merged)
		if err != nil {
			goDir.Remove()

			return nil, fmt.Errorf("loading plugin: %w", err)
		}

		loaded = append(loaded, p)
	}

	if len(loaded) == 0 {
		goDir.Remove()

		return nil, nil
	}

	return &Cache{plugins: loaded, goPath: goPath}, nil
}

// Tools returns all tool exports from loaded plugins.
func (c *Cache) Tools() []*PluginTool {
	if c == nil {
		return nil
	}

	var tools []*PluginTool

	for _, p := range c.plugins {
		if p.tool != nil {
			tools = append(tools, p.tool)
		}
	}

	return tools
}

// Commands returns all command exports from loaded plugins as slash commands.
func (c *Cache) Commands() []slash.Command {
	if c == nil {
		return nil
	}

	var cmds []slash.Command

	for _, p := range c.plugins {
		if p.command != nil {
			cmds = append(cmds, p.command.ToSlashCommand())
		}
	}

	return cmds
}

// matchesFilters checks whether a plugin name passes include/exclude filters.
// Supports wildcard patterns (e.g. "my-*") via wildcard.MatchAny, consistent
// with tool, MCP, and guardrail filtering.
func matchesFilters(name string, include, exclude []string) bool {
	if len(include) > 0 && !wildcard.MatchAny(name, include...) {
		return false
	}

	return !wildcard.MatchAny(name, exclude...)
}

// Hooks returns all hook injectors from all loaded plugins.
func (c *Cache) Hooks() []injector.Injector {
	if c == nil {
		return nil
	}

	var hooks []injector.Injector

	for _, p := range c.plugins {
		for _, h := range p.Hooks() {
			hooks = append(hooks, h)
		}
	}

	return hooks
}

// mergePluginConfig merges three config layers with override semantics:
// plugin.yaml defaults (lowest) → global features config → local per-plugin config (highest).
// Returns nil when all sources are empty.
func mergePluginConfig(pluginDefaults, global, local map[string]any) (map[string]any, error) {
	if len(pluginDefaults) == 0 && len(global) == 0 && len(local) == 0 {
		return nil, nil
	}

	merged := make(map[string]any)

	if err := merge.Merge(&merged, pluginDefaults); err != nil {
		return nil, fmt.Errorf("plugin defaults: %w", err)
	}

	if err := merge.Merge(&merged, global); err != nil {
		return nil, fmt.Errorf("global config: %w", err)
	}

	if err := merge.Merge(&merged, local); err != nil {
		return nil, fmt.Errorf("local config: %w", err)
	}

	if len(merged) == 0 {
		return nil, nil
	}

	return merged, nil
}

// Close releases all plugin interpreters and removes the shared temp GOPATH.
func (c *Cache) Close() {
	if c == nil {
		return
	}

	for _, p := range c.plugins {
		p.Close()
	}

	folder.New(c.goPath).Remove()
}
