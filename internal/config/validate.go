package config

import (
	"fmt"

	"github.com/go-playground/validator/v10"

	"github.com/idelchi/aura/internal/condition"

	"go.yaml.in/yaml/v4"
)

// Validate checks loaded config items for missing required fields and invalid values.
// Only parts present in the loaded set are validated.
func (c Config) Validate(loaded map[Part]struct{}) error {
	validate := validator.New(validator.WithRequiredStructEnabled())

	has := func(p Part) bool {
		_, ok := loaded[p]

		return ok
	}

	if has(PartAgents) {
		for file, agent := range c.Agents {
			if err := validate.Struct(agent.Metadata); err != nil {
				return fmt.Errorf("agent %s: %w", file, err)
			}
		}
	}

	if has(PartModes) {
		for file, mode := range c.Modes {
			if err := validate.Struct(mode.Metadata); err != nil {
				return fmt.Errorf("mode %s: %w", file, err)
			}
		}
	}

	if has(PartSystems) {
		for file, system := range c.Systems {
			if err := validate.Struct(system.Metadata); err != nil {
				return fmt.Errorf("system prompt %s: %w", file, err)
			}
		}
	}

	if has(PartProviders) {
		for name, provider := range c.Providers {
			if err := validate.Struct(provider); err != nil {
				return fmt.Errorf("provider %q: %w", name, err)
			}
		}
	}

	if has(PartCommands) {
		for file, cmd := range c.Commands {
			if err := validate.Struct(cmd.Metadata); err != nil {
				return fmt.Errorf("command %s: %w", file, err)
			}
		}
	}

	if has(PartSkills) {
		for file, skill := range c.Skills {
			if err := validate.Struct(skill.Metadata); err != nil {
				return fmt.Errorf("skill %s: %w", file, err)
			}
		}
	}

	if has(PartHooks) {
		for name, hook := range c.Hooks {
			if err := validate.Struct(hook); err != nil {
				return fmt.Errorf("hook %q: %w", name, err)
			}
		}
	}

	if has(PartFeatures) {
		if err := validate.Struct(c.Features); err != nil {
			return fmt.Errorf("features: %w", err)
		}
	}

	if has(PartMCPs) {
		for name, server := range c.MCPs {
			if err := validate.Struct(server); err != nil {
				return fmt.Errorf("mcp server %q: %w", name, err)
			}

			if server.Condition != "" {
				if err := condition.Validate(server.Condition); err != nil {
					return fmt.Errorf("mcp server %q condition: %w", name, err)
				}
			}
		}
	}

	if has(PartPlugins) {
		for name, plugin := range c.Plugins {
			if plugin.Condition != "" {
				if err := condition.Validate(plugin.Condition); err != nil {
					return fmt.Errorf("plugin %q condition: %w", name, err)
				}
			}
		}
	}

	if has(PartLSP) {
		for name, server := range c.LSPServers {
			if err := validate.Struct(server); err != nil {
				return fmt.Errorf("lsp server %q: %w", name, err)
			}
		}
	}

	if has(PartToolDefs) {
		for name, def := range c.ToolDefs {
			if err := validate.Struct(def); err != nil {
				return fmt.Errorf("tool definition %q: %w", name, err)
			}

			if def.Condition != "" {
				if err := condition.Validate(def.Condition); err != nil {
					return fmt.Errorf("tool definition %q condition: %w", name, err)
				}
			}
		}
	}

	if has(PartApprovalRules) {
		if err := validate.Struct(c.ApprovalRules); err != nil {
			return fmt.Errorf("approval rules: %w", err)
		}
	}

	if has(PartAgentsMd) {
		for file, md := range c.AgentsMd {
			if err := validate.Struct(md); err != nil {
				return fmt.Errorf("agents.md %s: %w", file, err)
			}
		}
	}

	return nil
}

// ValidateCrossRefs checks that all name-based references between config entities resolve
// to existing entries. Only checks cross-refs where both source and target parts are loaded.
func (c Config) ValidateCrossRefs(loaded map[Part]struct{}) error {
	has := func(p Part) bool {
		_, ok := loaded[p]

		return ok
	}

	if has(PartAgents) {
		for _, ag := range c.Agents {
			if has(PartSystems) && ag.Metadata.System != "" && c.Systems.Get(ag.Metadata.System) == nil {
				return fmt.Errorf("agent %q: system prompt %q not found", ag.Metadata.Name, ag.Metadata.System)
			}

			if has(PartModes) && ag.Metadata.Mode != "" && c.Modes.Get(ag.Metadata.Mode) == nil {
				return fmt.Errorf("agent %q: mode %q not found", ag.Metadata.Name, ag.Metadata.Mode)
			}

			if has(PartProviders) && ag.Metadata.Model.Provider != "" &&
				c.Providers.Get(ag.Metadata.Model.Provider) == nil {
				return fmt.Errorf("agent %q: provider %q not found", ag.Metadata.Name, ag.Metadata.Model.Provider)
			}

			for _, fb := range ag.Metadata.Fallback {
				if c.Agents.Get(fb) == nil {
					return fmt.Errorf("agent %q: fallback agent %q not found", ag.Metadata.Name, fb)
				}
			}

			if err := c.ValidateFeatureCrossRefs(
				fmt.Sprintf("agent %q", ag.Metadata.Name),
				ag.Metadata.Features,
				loaded,
			); err != nil {
				return err
			}
		}
	}

	if has(PartTasks) {
		for name, t := range c.Tasks {
			if has(PartAgents) && t.Agent != "" && c.Agents.Get(t.Agent) == nil {
				return fmt.Errorf("task %q: agent %q not found", name, t.Agent)
			}

			if has(PartModes) && t.Mode != "" && c.Modes.Get(t.Mode) == nil {
				return fmt.Errorf("task %q: mode %q not found", name, t.Mode)
			}

			if t.Features.Kind != 0 {
				var taskFeatures Features
				if err := t.Features.Load(&taskFeatures, yaml.WithKnownFields()); err != nil {
					return fmt.Errorf("task %q features: %w", name, err)
				}

				if err := c.ValidateFeatureCrossRefs(fmt.Sprintf("task %q", name), taskFeatures, loaded); err != nil {
					return err
				}
			}
		}
	}

	if has(PartFeatures) {
		return c.ValidateFeatureCrossRefs("", c.Features, loaded)
	}

	return nil
}

// ValidateFeatureCrossRefs checks all agent/prompt name references within a Features struct.
// owner identifies the source for error messages: "" for global, agent name for agents,
// or 'task "name"' for tasks. Checks are gated on whether the target part is loaded.
func (c Config) ValidateFeatureCrossRefs(owner string, f Features, loaded map[Part]struct{}) error {
	has := func(p Part) bool {
		_, ok := loaded[p]

		return ok
	}

	hasAgents := has(PartAgents)
	hasSystems := has(PartSystems)

	if !hasAgents && !hasSystems {
		return nil
	}

	prefix := func(field string) string {
		if owner == "" {
			return "features." + field
		}

		return owner + " features." + field
	}

	checks := []struct {
		field   string
		name    string
		isAgent bool
	}{
		{prefix("compaction.agent"), f.Compaction.Agent, true},
		{prefix("compaction.prompt"), f.Compaction.Prompt, false},
		{prefix("title.agent"), f.Title.Agent, true},
		{prefix("thinking.agent"), f.Thinking.Agent, true},
		{prefix("thinking.prompt"), f.Thinking.Prompt, false},
		{prefix("vision.agent"), f.Vision.Agent, true},
		{prefix("stt.agent"), f.STT.Agent, true},
		{prefix("tts.agent"), f.TTS.Agent, true},
		{prefix("embeddings.agent"), f.Embeddings.Agent, true},
		{prefix("embeddings.reranking.agent"), f.Embeddings.Reranking.Agent, true},
		{prefix("subagent.default_agent"), f.Subagent.DefaultAgent, true},
		{prefix("guardrail.scope.tool_calls.agent"), f.Guardrail.Scope.ToolCalls.Agent, true},
		{prefix("guardrail.scope.tool_calls.prompt"), f.Guardrail.Scope.ToolCalls.Prompt, false},
		{prefix("guardrail.scope.user_messages.agent"), f.Guardrail.Scope.UserMessages.Agent, true},
		{prefix("guardrail.scope.user_messages.prompt"), f.Guardrail.Scope.UserMessages.Prompt, false},
	}

	for _, ch := range checks {
		if ch.name == "" {
			continue
		}

		if ch.isAgent && hasAgents {
			if c.Agents.Get(ch.name) == nil {
				return fmt.Errorf("%s: agent %q not found", ch.field, ch.name)
			}
		} else if !ch.isAgent && hasSystems {
			if c.Systems.Get(ch.name) == nil {
				return fmt.Errorf("%s: system prompt %q not found", ch.field, ch.name)
			}
		}
	}

	return nil
}
