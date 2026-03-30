package config

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/files"
)

// ToolDef is a text-only override for a built-in or plugin tool.
// Loaded from .aura/config/tools/**/*.yaml.
type ToolDef struct {
	Disabled    *bool  `yaml:"disabled"`
	Override    bool   `yaml:"override"`
	Description string `yaml:"description"`
	Usage       string `yaml:"usage"`
	Examples    string `yaml:"examples"`
	Condition   string `yaml:"condition"` // condition expression for conditional inclusion
	Parallel    *bool  `yaml:"parallel"`  // overrides tool.Parallel()
	Source      string `yaml:"-"`         // file path this definition was loaded from
}

// IsDisabled returns true if the tool definition is explicitly disabled.
func (d ToolDef) IsDisabled() bool { return d.Disabled != nil && *d.Disabled }

// IsEnabled returns true if the tool definition is active (not explicitly disabled).
func (d ToolDef) IsEnabled() bool { return !d.IsDisabled() }

// Text returns the text fields for use as a text-only override.
func (d ToolDef) Text() tool.Text {
	return tool.Text{
		Description: d.Description,
		Usage:       d.Usage,
		Examples:    d.Examples,
	}
}

// ToolDefs maps tool names to their definitions.
type ToolDefs map[string]ToolDef

// Apply applies text overrides and removes disabled tools.
// Matches tools by lowercased name against the ToolDefs keys.
// MergeText is idempotent — safe to call multiple times on the same tool set.
func (td ToolDefs) Apply(ts *tool.Tools) {
	type merger interface {
		MergeText(tool.Text)
	}

	var removals []string

	for _, t := range *ts {
		def, ok := td[strings.ToLower(t.Name())]
		if !ok {
			continue
		}

		if def.IsDisabled() {
			removals = append(removals, t.Name())

			continue
		}

		if m, ok := t.(merger); ok {
			m.MergeText(def.Text())
		}
	}

	for _, name := range removals {
		ts.Remove(name)
	}
}

// ParallelOverride returns the config-level parallel override for a tool, or nil if not set.
func (td ToolDefs) ParallelOverride(name string) *bool {
	def, ok := td[strings.ToLower(name)]
	if !ok {
		return nil
	}

	return def.Parallel
}

// Load reads YAML files and merges all tool definitions into the map.
// Each file contains a map of tool names to their definitions.
// Multiple tools can live in one file, or each tool in its own file —
// the loader is decoupled from filenames.
func (td *ToolDefs) Load(ff files.Files) error {
	*td = make(ToolDefs)

	for _, f := range ff {
		content, err := f.Read()
		if err != nil {
			return fmt.Errorf("reading tool definition %s: %w", f, err)
		}

		var file map[string]ToolDef
		if err := yamlutil.StrictUnmarshal(content, &file); err != nil {
			return fmt.Errorf("parsing tool definition %s: %w", f, err)
		}

		for name, def := range file {
			def.Source = f.Path()
			(*td)[strings.ToLower(name)] = def
		}
	}

	return nil
}
