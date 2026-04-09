package config

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/config/inherit"
	"github.com/idelchi/aura/internal/prompts"
	"github.com/idelchi/aura/pkg/frontmatter"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"

	"go.yaml.in/yaml/v4"
)

// loadSystems parses system prompt Markdown files and returns a Collection keyed by source file.
// Supports config inheritance via the `inherit` frontmatter field.
// Unlike agents/modes (which replace bodies), prompts concatenate:
// parent bodies are prepended in order, building up the final prompt.
func loadSystems(ff files.Files) (Collection[System], error) {
	// Phase A: Parse frontmatter into structs, collect bodies.
	metas := make(map[string]SystemMetadata)
	bodies := make(map[string]string)
	fileMap := make(map[string]file.File)

	for _, f := range ff {
		yamlBytes, body, err := frontmatter.LoadRaw(f)
		if err != nil {
			return nil, fmt.Errorf("system prompt %s: %w", f, err)
		}

		var meta SystemMetadata
		if err := yaml.Load(yamlBytes, &meta, yaml.WithKnownFields()); err != nil {
			return nil, fmt.Errorf("system prompt %s: %w", f, err)
		}

		if meta.Name == "" {
			return nil, fmt.Errorf("system prompt %s: missing required 'name' field", f)
		}

		name := meta.Name

		dedupByName(name, metas, bodies, fileMap)

		metas[name] = meta
		bodies[name] = body
		fileMap[name] = f
	}

	// Phase B: Resolve inheritance (struct-level merge).
	resolved, err := inherit.Resolve(metas, func(m SystemMetadata) []string {
		return m.Inherit
	})
	if err != nil {
		return nil, err
	}

	// Phase C: Compose systems from merged metadata + body concatenation.
	// Unlike agents/modes (empty body = inherit, non-empty = replace),
	// prompts concatenate: direct parent bodies first, then child body.
	result := make(Collection[System])

	for name, meta := range resolved {
		var parts []string

		for _, p := range metas[name].Inherit {
			if bodies[p] != "" {
				parts = append(parts, bodies[p])
			}
		}

		if bodies[name] != "" {
			parts = append(parts, bodies[name])
		}

		result[fileMap[name]] = System{
			Metadata: meta,
			Prompt:   prompts.Prompt(strings.Join(parts, "\n\n")),
		}
	}

	return result, nil
}

// SystemMetadata holds the YAML-decoded configuration for a system prompt.
type SystemMetadata struct {
	// Name is the unique identifier for the system prompt.
	Name string `validate:"required"`
	// Description explains the system prompt's purpose.
	Description string
	// Inherit lists parent prompt names whose bodies are prepended to this prompt.
	Inherit []string `yaml:"inherit"`
}

// System represents a system prompt configuration.
type System struct {
	// Metadata contains system prompt configuration details.
	Metadata SystemMetadata
	// Prompt is the system prompt template.
	Prompt prompts.Prompt
}

// Name returns the system prompt's identifier.
func (s System) Name() string { return s.Metadata.Name }
