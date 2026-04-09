package agent

import (
	"errors"
	"fmt"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/responseformat"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// Agent represents a running LLM agent with all its runtime state.
type Agent struct {
	Name           string
	Model          config.Model
	Provider       providers.Provider
	Mode           string
	System         string // system prompt name (empty = agent default or default compositor)
	Tools          tool.Tools
	Prompt         string // rendered system prompt
	Thinking       thinking.Strategy
	ResponseFormat *responseformat.ResponseFormat
}

// Overrides holds CLI flag overrides applied on top of agent config.
// Pointer fields use nil = no override, non-nil = explicit value (including empty to clear).
type Overrides struct {
	Agent      *string // nil = no override (use session/default agent); for ResumeSession branching only
	Mode       *string
	System     *string
	Think      *thinking.Value
	Provider   *string
	Model      *string
	Context    *int                   // --override model.context
	Generation *generation.Generation // --override model.generation.*
	Vars       map[string]string      // --set template variables, threaded to BuildAgent
}

// New creates a new Agent from configuration with optional overrides.
func New(
	cfg config.Config,
	paths config.Paths,
	rt *config.Runtime,
	name string,
	overrides ...Overrides,
) (*Agent, error) {
	agentConfig := cfg.Agents.Get(name)
	if agentConfig == nil {
		return nil, fmt.Errorf("agent %q not found", name)
	}

	providerConfig := cfg.Providers.Get(agentConfig.Metadata.Model.Provider)
	if providerConfig == nil {
		return nil, fmt.Errorf("provider %q not found", agentConfig.Metadata.Model.Provider)
	}

	provider, err := providers.New(*providerConfig)
	if err != nil {
		return nil, err
	}

	var mode *string // nil = use agent default in BuildAgent

	var system *string // nil = use agent default in BuildAgent

	model := agentConfig.Metadata.Model

	// Apply overrides from CLI flags.
	if len(overrides) > 0 {
		o := overrides[0]

		debug.Log("[agent] applying overrides for %s", name)

		if o.Mode != nil {
			if *o.Mode != "" {
				if cfg.Modes.Get(*o.Mode) == nil {
					return nil, fmt.Errorf("mode %q not found", *o.Mode)
				}
			}

			mode = o.Mode
		}

		if o.System != nil {
			if *o.System != "" {
				if cfg.Systems.Get(*o.System) == nil {
					return nil, fmt.Errorf("system prompt %q not found", *o.System)
				}
			}

			system = o.System
		}

		if o.Think != nil {
			model.Think = *o.Think
		}

		if o.Provider != nil {
			if *o.Provider == "" {
				return nil, errors.New("provider cannot be empty")
			}

			providerConfig = cfg.Providers.Get(*o.Provider)
			if providerConfig == nil {
				return nil, fmt.Errorf("provider %q not found", *o.Provider)
			}

			provider, err = providers.New(*providerConfig)
			if err != nil {
				return nil, err
			}

			model.Provider = *o.Provider
		}

		if o.Model != nil {
			if *o.Model == "" {
				return nil, errors.New("model cannot be empty")
			}

			model.Name = *o.Model
		}

		if o.Context != nil {
			model.Context = *o.Context
		}

		if o.Generation != nil {
			if model.Generation == nil {
				model.Generation = &generation.Generation{}
			}

			model.Generation.Merge(o.Generation)
		}
	}

	// Resolve mode: nil = use agent default, non-nil = explicit (including "" for no mode)
	modeStr := agentConfig.Metadata.Mode

	if mode != nil {
		modeStr = *mode
	}

	// Resolve system: nil = use agent default (resolved inside BuildAgent), non-nil = explicit
	systemStr := ""

	if system != nil {
		systemStr = *system
	}

	var vars map[string]string

	if len(overrides) > 0 {
		vars = overrides[0].Vars
	}

	// Merge agent and mode features into the local cfg copy so BuildAgent
	// reads correct sandbox state, restrictions, and other feature overrides.
	// cfg is by-value, so this doesn't affect the caller's copy.
	resolved, err := cfg.ResolveFeatures(cfg.Features, name, modeStr)
	if err != nil {
		return nil, fmt.Errorf("resolving features: %w", err)
	}

	cfg.Features = resolved

	prompt, _, tools, _, err := cfg.BuildAgent(name, modeStr, systemStr, config.TemplateData{
		Provider: model.Provider,
		Agent:    name,
		Mode:     config.ModeData{Name: modeStr},
		Vars:     config.ToAnyMap(vars),
		Model:    config.ModelData{Name: model.Name},
	}, paths, rt)
	if err != nil {
		return nil, err
	}

	debug.Log(
		"[agent] created %s (provider=%s model=%s mode=%s tools=%d)",
		name,
		model.Provider,
		model.Name,
		modeStr,
		len(tools),
	)

	return &Agent{
		Name:           name,
		Model:          model,
		Provider:       provider,
		Mode:           modeStr,
		System:         systemStr,
		Tools:          tools,
		Prompt:         prompt.String(),
		Thinking:       agentConfig.Metadata.Thinking,
		ResponseFormat: agentConfig.Metadata.ResponseFormat,
	}, nil
}
