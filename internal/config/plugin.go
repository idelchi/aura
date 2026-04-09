package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/gitutil"
	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Plugin is a user-defined plugin loaded from .aura/plugins/<name>/plugin.yaml.
type Plugin struct {
	Description string         `yaml:"description"`
	Condition   string         `yaml:"condition"`
	Disabled    *bool          `yaml:"disabled"`
	Once        bool           `yaml:"once"`
	Override    bool           `yaml:"override"` // replace a built-in tool with the same name
	OptIn       bool           `yaml:"opt_in"`   // tool is hidden unless explicitly enabled by name
	Env         []string       `yaml:"env"`      // env var names this plugin may read (["*"] = all)
	Config      map[string]any `yaml:"config"`   // default config values shipped with the plugin

	Name      string         `yaml:"name"` // optional; defaults to directory name in Load()
	Source    string         `yaml:"-"`    // plugin.yaml file path
	Origin    gitutil.Origin `yaml:"-"`    // loaded from .origin.yaml sidecar
	OriginDir string         `yaml:"-"`    // directory where .origin.yaml lives (may differ from Dir() for packs)
}

// IsEnabled returns whether the plugin is active. Default is true (nil Disabled = enabled).
func (p Plugin) IsEnabled() bool { return p.Disabled == nil || !*p.Disabled }

// Dir returns the directory containing the plugin .go files.
func (p Plugin) Dir() string { return folder.FromFile(file.New(p.Source)).Path() }

// HasOrigin returns true if the plugin was installed from a git source.
func (p Plugin) HasOrigin() bool { return p.Origin.URL != "" }

// Display returns a one-line summary for listing.
// Capabilities are provided externally because discovery requires a Yaegi interpreter.
func (p Plugin) Display(hooks []string, toolName, commandName string) string {
	status := "enabled"

	if !p.IsEnabled() {
		status = "disabled"
	}

	var parts []string

	if len(hooks) > 0 {
		parts = append(parts, strings.Join(hooks, ","))
	}

	if toolName != "" {
		parts = append(parts, "tool:"+toolName)
	}

	if commandName != "" {
		parts = append(parts, "cmd:/"+commandName)
	}

	capsStr := strings.Join(parts, " | ")
	if capsStr == "" {
		capsStr = "-"
	}

	source := p.Source
	if p.HasOrigin() {
		short := p.Origin.URL

		short = strings.TrimPrefix(short, "https://")
		short = strings.TrimPrefix(short, "http://")

		ref := p.Origin.Ref
		if ref == "" {
			ref = gitutil.ShortCommit(p.Origin.Commit)
		}

		if ref != "" {
			source += fmt.Sprintf(" (git: %s@%s)", short, ref)
		} else {
			source += fmt.Sprintf(" (git: %s)", short)
		}
	}

	return fmt.Sprintf("%-20s %-8s  %-35s  %s", p.Name, status, capsStr, source)
}

// IsPack returns true if the plugin is part of a multi-plugin pack (OriginDir != Dir()).
func (p Plugin) IsPack() bool {
	return p.OriginDir != "" && p.OriginDir != p.Dir()
}

// PackName returns the pack directory name, or empty string if not a pack member.
func (p Plugin) PackName() string {
	if !p.IsPack() {
		return ""
	}

	return folder.New(p.OriginDir).Base()
}

// ByPack returns all plugins whose OriginDir base name matches the given pack name, sorted by plugin name.
func ByPack(pp StringCollection[Plugin], packName string) []Plugin {
	var result []Plugin

	for _, name := range pp.Names() {
		p := pp[name]
		if p.OriginDir != "" && folder.New(p.OriginDir).Base() == packName {
			result = append(result, p)
		}
	}

	return result
}

// PackNames returns a sorted slice of unique pack directory names.
func PackNames(pp StringCollection[Plugin]) []string {
	seen := map[string]bool{}

	var names []string

	for _, name := range pp.Names() {
		p := pp[name]
		if p.IsPack() {
			packName := folder.New(p.OriginDir).Base()
			if !seen[packName] {
				seen[packName] = true
				names = append(names, packName)
			}
		}
	}

	slices.Sort(names)

	return names
}

// loadPlugins reads plugin.yaml files and returns a StringCollection of plugins.
// The plugin name comes from the YAML "name" field if present, otherwise from the parent directory name.
// Origin is loaded from the sidecar .origin.yaml if present.
func loadPlugins(ff files.Files) (StringCollection[Plugin], error) {
	result := make(StringCollection[Plugin])

	for _, f := range ff {
		content, err := f.Read()
		if err != nil {
			return nil, fmt.Errorf("reading plugin %s: %w", f, err)
		}

		var p Plugin
		if err := yamlutil.StrictUnmarshal(content, &p); err != nil {
			return nil, fmt.Errorf("parsing plugin %s: %w", f, err)
		}

		p.Source = f.Path()

		name := p.Name
		if name == "" {
			name = folder.FromFile(f).Base()
		}

		p.Name = name

		// Load origin from sidecar if present.
		// Walk up from plugin dir to find .origin.yaml (handles packs and subpath installs).
		if origin, originDir, found := gitutil.FindOrigin(p.Dir()); found {
			// Skip plugins outside origin subpath scope.
			if len(origin.Subpaths) > 0 {
				pluginDir := folder.New(p.Dir())
				inScope := false

				for _, sp := range origin.Subpaths {
					scopedRoot := folder.New(originDir, sp)

					rel, err := pluginDir.RelativeTo(scopedRoot)
					if err == nil && !strings.HasPrefix(rel.Path(), "..") {
						inScope = true

						break
					}
				}

				if !inScope {
					debug.Log("[plugins] skipping %s: outside subpath scope", pluginDir)

					continue
				}
			}

			p.Origin = origin
			p.OriginDir = originDir
		}

		result[strings.ToLower(name)] = p
	}

	return result, nil
}
