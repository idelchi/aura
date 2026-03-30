package assistant

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/pkg/llm/model"
)

// FeatureResolution holds the resolved provider/model/prompt for a secondary LLM call
// (compaction, thinking rewrite, guardrail check).
type FeatureResolution struct {
	provider   providers.Provider
	mdl        model.Model
	contextLen int
	prompt     string
}

// FeatureAgentConfig holds the inputs for resolving a feature agent.
type FeatureAgentConfig struct {
	label      string        // debug log label ("compact", "thinking", "guardrail-tool_calls")
	promptName string        // config's .prompt field (named prompt for self-use)
	agentName  string        // config's .agent field (dedicated agent name)
	modelCache **model.Model // pointer to the model cache field for lazy resolution
}

// ResolveAgent determines which provider, model, and system prompt to use for a
// feature agent call.
//
// Resolution order:
//  1. promptName set → self-use: current agent's model + named prompt from prompts/
//  2. agentName set → create dedicated agent on demand
//  3. Error: neither set → "no agent available"
func (a *Assistant) ResolveAgent(ctx context.Context, cfg FeatureAgentConfig) (FeatureResolution, error) {
	// Case 1: self-use with a named prompt.
	if cfg.promptName != "" {
		sys := a.cfg.Systems.Get(cfg.promptName)
		if sys == nil {
			return FeatureResolution{}, fmt.Errorf("%s prompt %q not found", cfg.label, cfg.promptName)
		}

		rendered, err := sys.Prompt.Render(a.TemplateData())
		if err != nil {
			return FeatureResolution{}, fmt.Errorf("rendering %s prompt %q: %w", cfg.label, cfg.promptName, err)
		}

		mdl, err := a.ResolveModel(ctx, cfg.label, a.agent, cfg.modelCache)
		if err != nil {
			return FeatureResolution{}, err
		}

		debug.Log("[%s] self-use: prompt=%q model=%s", cfg.label, cfg.promptName, mdl.Name)

		return FeatureResolution{
			provider:   a.agent.Provider,
			mdl:        mdl,
			contextLen: a.agent.Model.Context,
			prompt:     rendered.String(),
		}, nil
	}

	// Case 2: dedicated agent created on demand.
	if cfg.agentName != "" {
		overrideAgent, err := agent.New(a.cfg, a.paths, a.rt, cfg.agentName)
		if err != nil {
			return FeatureResolution{}, fmt.Errorf("creating %s agent %q: %w", cfg.label, cfg.agentName, err)
		}

		a.applyNoop(overrideAgent)

		mdl, err := a.ResolveModel(ctx, cfg.label, overrideAgent, cfg.modelCache)
		if err != nil {
			return FeatureResolution{}, err
		}

		debug.Log("[%s] dedicated agent: %q model=%s", cfg.label, cfg.agentName, mdl.Name)

		return FeatureResolution{
			provider:   overrideAgent.Provider,
			mdl:        mdl,
			contextLen: overrideAgent.Model.Context,
			prompt:     overrideAgent.Prompt,
		}, nil
	}

	return FeatureResolution{}, fmt.Errorf(
		"no %s agent available (set %s.agent or %s.prompt)",
		cfg.label,
		cfg.label,
		cfg.label,
	)
}
