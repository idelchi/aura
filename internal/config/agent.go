package config

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/config/inherit"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/prompts"
	"github.com/idelchi/aura/pkg/frontmatter"
	"github.com/idelchi/aura/pkg/llm/responseformat"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"

	"go.yaml.in/yaml/v4"
)

// AgentMetadata holds the YAML-decoded configuration for an agent.
type AgentMetadata struct {
	// Name is the unique identifier for the agent.
	Name string `validate:"required"`
	// Description explains the agent's purpose.
	Description string
	// Model specifies which AI model to use.
	Model Model `validate:"required"`
	// Tools defines tool availability for this agent.
	Tools Tools
	// Hooks controls which hooks are active for this agent.
	Hooks HookFilter
	// System is the name of the system prompt to use.
	System string
	// Mode is the operational mode for this agent.
	Mode string
	// Hide excludes this agent from UI listing and cycling.
	Hide *bool
	// Default marks this agent as the default when --agent is not specified.
	// Only one loaded agent may have Default: true (error if multiple).
	Default *bool `yaml:"default"`
	// Subagent marks this agent as available for the Task tool (subagent delegation).
	Subagent *bool `yaml:"subagent"`
	// AgentsMd controls which AGENTS.md files are injected into this agent's prompt.
	// Values: "" or "all" (default), "global", "local", "none".
	AgentsMd AgentsMdFilter `validate:"omitempty,oneof=all global local none" yaml:"agentsmd"`
	// Thinking controls how thinking blocks from prior turns are handled.
	Thinking thinking.Strategy
	// Files lists paths to load into the system prompt at BuildAgent() time.
	// Paths are expanded as Go templates before reading (e.g., {{ .Config.Source }}/file.md).
	// Relative paths are resolved against the config home directory.
	Files []string
	// SourceHome is the config home directory this agent was loaded from.
	// Set during Load() by matching the agent file's path against the homes list.
	SourceHome string `yaml:"-"`
	// Features holds per-agent feature overrides. Non-zero values are merged
	// on top of global feature defaults at agent resolution time.
	Features Features
	// Inherit lists parent agent names for config inheritance.
	Inherit []string `yaml:"inherit"`
	// Fallback lists agent names to try when this agent's provider fails.
	// Ordered: first entry is tried first. Evaluated only on the primary agent —
	// fallback agents' own fallback lists are ignored.
	Fallback []string `yaml:"fallback"`
	// ResponseFormat constrains LLM responses to a specific format (nil = unconstrained text).
	ResponseFormat *responseformat.ResponseFormat `yaml:"response_format"`
}

// IsHidden returns true if the agent is excluded from UI listing.
func (m AgentMetadata) IsHidden() bool { return m.Hide != nil && *m.Hide }

// IsDefault returns true if the agent is marked as the default.
func (m AgentMetadata) IsDefault() bool { return m.Default != nil && *m.Default }

// IsSubagent returns true if the agent is available for the Task tool.
func (m AgentMetadata) IsSubagent() bool { return m.Subagent != nil && *m.Subagent }

// Agent represents the configuration and prompt of an agent.
type Agent struct {
	// Metadata contains agent configuration details.
	Metadata AgentMetadata
	// Prompt is the agent's prompt template.
	Prompt prompts.Prompt
}

// Name returns the agent's name, satisfying the Namer interface.
func (a Agent) Name() string { return a.Metadata.Name }

// Visible returns true if the agent is not hidden, for use as a Filter predicate.
func (a Agent) Visible() bool { return !a.Metadata.IsHidden() }

// IsSubagent returns true if the agent is available for the Task tool, for use as a Filter predicate.
func (a Agent) IsSubagent() bool { return a.Metadata.IsSubagent() }

// IsDefault returns true if the agent is marked as the default, for use as a Filter predicate.
func (a Agent) IsDefault() bool { return a.Metadata.IsDefault() }

// HasProvider returns true if the agent's provider is in the given list (case-insensitive),
// or if the agent has no provider configured.
func (a Agent) HasProvider(providers []string) bool {
	provider := a.Metadata.Model.Provider
	if provider == "" {
		return true
	}

	for _, p := range providers {
		if strings.EqualFold(provider, p) {
			return true
		}
	}

	return false
}

// ResolveDefault finds the default agent using the following precedence:
//  1. The agent with Default: true (error if multiple).
//  2. The first non-hidden agent (alphabetical via Names).
//  3. Error if no agents are available.
func ResolveDefault(agents Collection[Agent], homes []string) (*Agent, error) {
	var defaultAgent *Agent

	for _, agent := range agents {
		if agent.Metadata.IsDefault() {
			if defaultAgent != nil {
				return nil, fmt.Errorf(
					"multiple agents marked as default: %q and %q",
					defaultAgent.Metadata.Name,
					agent.Metadata.Name,
				)
			}

			found := agent

			defaultAgent = &found
		}
	}

	if defaultAgent != nil {
		return defaultAgent, nil
	}

	for _, name := range agents.Filter(Agent.Visible).Names() {
		return agents.Get(name), nil
	}

	return nil, fmt.Errorf(
		"no agents available — add agent definitions to config/agents/ in one of the config homes: %s",
		strings.Join(homes, ", "),
	)
}

// homeFor returns the config home that owns the given file path.
// Homes are checked in reverse order (most specific match wins).
func homeFor(filePath string, homes []string) string {
	for i := len(homes) - 1; i >= 0; i-- {
		if strings.HasPrefix(filePath, homes[i]) {
			return homes[i]
		}
	}

	return ""
}

// loadAgents parses agent files into a Collection, resolving inheritance.
// Supports config inheritance via the `inherit` frontmatter field.
// Homes are used to determine each agent's source config directory.
func loadAgents(ff files.Files, homes []string) (Collection[Agent], error) {
	result := make(Collection[Agent])

	// Phase A: Parse frontmatter into structs, collect bodies.
	metas := make(map[string]AgentMetadata)
	bodies := make(map[string]string)
	fileMap := make(map[string]file.File)

	for _, f := range ff {
		yamlBytes, body, err := frontmatter.LoadRaw(f)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", f, err)
		}

		var meta AgentMetadata
		if err := yaml.Load(yamlBytes, &meta, yaml.WithKnownFields()); err != nil {
			return nil, fmt.Errorf("agent %s: %w", f, err)
		}

		if meta.Name == "" {
			return nil, fmt.Errorf("agent %s: missing required 'name' field", f)
		}

		meta.SourceHome = homeFor(f.Path(), homes)

		name := meta.Name

		dedupByName(name, metas, bodies, fileMap)

		metas[name] = meta
		bodies[name] = body
		fileMap[name] = f
	}

	// Phase B: Resolve inheritance (struct-level merge).
	resolved, err := inherit.Resolve(metas, func(m AgentMetadata) []string {
		return m.Inherit
	})
	if err != nil {
		return nil, err
	}

	// Default is an identity property ("this agent is the default"), not a
	// configuration property. Inheriting it would cause "multiple defaults" errors
	// whenever a parent with default: true has children. Restore from pre-merge values.
	for name, meta := range resolved {
		meta.Default = metas[name].Default
		resolved[name] = meta
	}

	// Phase C: Compose agents from merged metadata + body inheritance.
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

		if body == "" {
			debug.Log("[config] agent %q resolved to empty prompt", name)
		}

		result[fileMap[name]] = Agent{
			Metadata: meta,
			Prompt:   prompts.Prompt(body),
		}
	}

	return result, nil
}
