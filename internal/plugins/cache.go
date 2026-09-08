package plugins

import (
	"fmt"
	"reflect"
	"slices"

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
	definitions config.StringCollection[config.Plugin] // Definitions retained for lazy selection.
	loaded      map[string]*Plugin                     // Interpreters retained across temporary exclusion.
	settings    map[string]loadSettings                // Initialization inputs for each loaded interpreter.
	active      []*Plugin                              // Current effective selection, sorted by plugin name.
	goPath      string                                 // Shared temporary dependency workspace.
	configDir   string                                 // Config home passed to plugin initialization.
}

// loadSettings identifies interpreter initialization inputs that require reloading.
type loadSettings struct {
	unsafe bool           // Access to restricted interpreter imports.
	config map[string]any // Merged default, global and per-plugin settings.
}

// LoadAll creates a Cache by loading all enabled plugins from config.
// Returns a nil Cache only when no plugins are configured. Inactive definitions
// remain available for later agent or mode selections without loading their code.
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

	c := &Cache{
		definitions: cfgPlugins,
		loaded:      make(map[string]*Plugin),
		settings:    make(map[string]loadSettings),
		goPath:      goPath,
		configDir:   configDir,
	}
	if _, err := c.Select(features); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// Select activates the effective plugin set. Unchanged interpreters keep their
// state, including across temporary exclusion. New code loads only when selected.
// A failed load leaves the previous selection and its interpreters intact.
func (c *Cache) Select(features config.PluginConfig) (bool, error) {
	if c == nil {
		return false, nil
	}
	var active []*Plugin
	created := make(map[string]*Plugin)
	settings := make(map[string]loadSettings)
	committed := false
	defer func() {
		if !committed {
			for _, p := range created {
				p.Close()
			}
		}
	}()
	for _, name := range c.definitions.Names() {
		cfg := c.definitions[name]
		if !cfg.IsEnabled() || !matchesFilters(name, features.Include, features.Exclude) {
			continue
		}
		merged, err := mergePluginConfig(cfg.Config, features.Config.Global, features.Config.Local[name])
		if err != nil {
			return false, fmt.Errorf("merging config for plugin %q: %w", name, err)
		}
		wanted := loadSettings{unsafe: features.Unsafe, config: merged}
		p := c.loaded[name]
		if p == nil || !reflect.DeepEqual(c.settings[name], wanted) {
			p, err = Load(name, cfg, c.goPath, features.Unsafe, c.configDir, merged)
			if err != nil {
				return false, fmt.Errorf("loading plugin: %w", err)
			}
			created[name] = p
			settings[name] = wanted
		}
		active = append(active, p)
	}
	changed := !slices.Equal(c.active, active)
	for name, p := range created {
		if old := c.loaded[name]; old != nil {
			old.Close()
		}
		c.loaded[name] = p
		c.settings[name] = settings[name]
	}
	c.active = active
	committed = true
	if changed {
		debug.Log("[plugin] selected %d plugins", len(active))
	}
	return changed, nil
}

// Tools returns tool exports from the active selection.
func (c *Cache) Tools() []*PluginTool {
	if c == nil {
		return nil
	}

	var tools []*PluginTool

	for _, p := range c.active {
		if p.tool != nil {
			tools = append(tools, p.tool)
		}
	}

	return tools
}

// Commands returns command exports from the active selection as slash commands.
func (c *Cache) Commands() []slash.Command {
	if c == nil {
		return nil
	}

	var cmds []slash.Command

	for _, p := range c.active {
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

// Hooks returns hook injectors from the active selection.
func (c *Cache) Hooks() []injector.Injector {
	if c == nil {
		return nil
	}

	var hooks []injector.Injector

	for _, p := range c.active {
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

	for _, p := range c.loaded {
		p.Close()
	}

	folder.New(c.goPath).Remove()
}
