package config

import (
	"fmt"

	"github.com/idelchi/aura/internal/config/inherit"
	"github.com/idelchi/aura/internal/prompts"
	"github.com/idelchi/aura/pkg/frontmatter"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"

	"go.yaml.in/yaml/v4"
)

// ModeMetadata holds the YAML-decoded configuration for a mode.
type ModeMetadata struct {
	// Name is the unique identifier for the mode.
	Name string `validate:"required"`
	// Description explains the mode's purpose.
	Description string
	// Hide excludes this mode from UI listing and cycling.
	Hide *bool
	// Tools defines tool availability for this mode.
	Tools Tools
	// Hooks controls which hooks are active for this mode.
	Hooks HookFilter
	// Features holds per-mode feature overrides. Non-zero values are merged
	// on top of agent features at mode resolution time.
	Features Features
	// Inherit lists parent mode names for config inheritance.
	Inherit []string `yaml:"inherit"`
}

// IsHidden returns true if the mode is excluded from UI listing.
func (m ModeMetadata) IsHidden() bool { return m.Hide != nil && *m.Hide }

// Mode represents an operational mode with associated tools and prompt.
type Mode struct {
	// Metadata contains mode configuration details.
	Metadata ModeMetadata
	// Prompt is the mode's prompt template.
	Prompt prompts.Prompt
}

// Name returns the mode's unique identifier.
func (m Mode) Name() string { return m.Metadata.Name }

// Visible reports whether the mode is included in UI listing and cycling.
func (m Mode) Visible() bool { return !m.Metadata.IsHidden() }

// loadModes parses mode files into a Collection, resolving inheritance.
func loadModes(ff files.Files) (Collection[Mode], error) {
	result := make(Collection[Mode])

	// Phase A: Parse frontmatter into structs, collect bodies.
	metas := make(map[string]ModeMetadata)
	bodies := make(map[string]string)
	fileMap := make(map[string]file.File)

	for _, f := range ff {
		yamlBytes, body, err := frontmatter.LoadRaw(f)
		if err != nil {
			return nil, fmt.Errorf("mode %s: %w", f, err)
		}

		var meta ModeMetadata
		if err := yaml.Load(yamlBytes, &meta, yaml.WithKnownFields()); err != nil {
			return nil, fmt.Errorf("mode %s: %w", f, err)
		}

		if meta.Name == "" {
			return nil, fmt.Errorf("mode %s: missing required 'name' field", f)
		}

		name := meta.Name

		dedupByName(name, metas, bodies, fileMap)

		metas[name] = meta
		bodies[name] = body
		fileMap[name] = f
	}

	// Phase B: Resolve inheritance (struct-level merge).
	resolved, err := inherit.Resolve(metas, func(m ModeMetadata) []string {
		return m.Inherit
	})
	if err != nil {
		return nil, err
	}

	// Phase C: Compose modes from merged metadata + body inheritance.
	for name, meta := range resolved {
		body := bodies[name]
		if body == "" {
			for _, p := range metas[name].Inherit {
				if bodies[p] != "" {
					body = bodies[p]

					break
				}
			}
		}

		result[fileMap[name]] = Mode{
			Metadata: meta,
			Prompt:   prompts.Prompt(body),
		}
	}

	return result, nil
}
