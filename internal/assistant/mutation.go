package assistant

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/condition"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/hooks"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/internal/tools/ask"
	"github.com/idelchi/aura/internal/tools/done"
	"github.com/idelchi/aura/internal/tools/loadtools"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/sandbox"
)

// SwitchAgent switches to a different agent (preserves conversation).
// If the new agent changes MCP filter rules, MCP sessions are
// closed and reconnected so the correct set of tools is active.
func (a *Assistant) SwitchAgent(name, reason string) error {
	oldInclude, oldExclude := a.cfg.MCPFilter(a.rt)

	overrides := a.cliOverrides

	overrides.Model = nil
	overrides.Provider = nil

	ag, err := agent.New(a.cfg, a.paths, a.rt, name, overrides)
	if err != nil {
		return err
	}

	prev := a.agent
	prevModel := a.resolved.model

	a.agent = ag
	a.applyNoop(ag)

	a.resolved.model = nil // reset model cache

	if err := a.rebuildState(); err != nil {
		a.agent = prev
		a.resolved.model = prevModel

		return err
	}

	// Fire OnAgentSwitch timing.
	state := a.InjectorState()

	state.AgentSwitch.Previous = prev.Name
	state.AgentSwitch.New = name
	state.AgentSwitch.Reason = reason
	a.injectMessages(a.tools.injectors.Run(a.ctx, injector.OnAgentSwitch, state))

	newInclude, newExclude := a.cfg.MCPFilter(a.rt)
	if !slices.Equal(oldInclude, newInclude) || !slices.Equal(oldExclude, newExclude) {
		a.closeMCPSessions()
		a.reconnectMCP(a.ctx)
	}

	return nil
}

// setAgentFailover switches to a fallback agent, stripping --model and --provider
// CLI overrides (they belong to the primary agent, not the fallback).
func (a *Assistant) setAgentFailover(name string) error {
	overrides := a.cliOverrides

	overrides.Model = nil
	overrides.Provider = nil

	ag, err := agent.New(a.cfg, a.paths, a.rt, name, overrides)
	if err != nil {
		return err
	}

	prev := a.agent
	prevModel := a.resolved.model

	a.agent = ag
	a.applyNoop(ag)

	a.resolved.model = nil

	if err := a.rebuildState(); err != nil {
		a.agent = prev
		a.resolved.model = prevModel

		return err
	}

	// Fire OnAgentSwitch timing.
	state := a.InjectorState()

	state.AgentSwitch.Previous = prev.Name
	state.AgentSwitch.New = name
	state.AgentSwitch.Reason = "failover"
	a.injectMessages(a.tools.injectors.Run(a.ctx, injector.OnAgentSwitch, state))

	return nil
}

// SwitchModel switches the model and provider mid-session (preserves conversation).
func (a *Assistant) SwitchModel(ctx context.Context, providerName, modelName string) error {
	providerCfg := a.cfg.Providers.Get(providerName)
	if providerCfg == nil {
		return fmt.Errorf("provider %q not found", providerName)
	}

	newProvider, err := providers.New(*providerCfg)
	if err != nil {
		return fmt.Errorf("creating provider %q: %w", providerName, err)
	}

	resolved, err := newProvider.Model(ctx, modelName)
	if err != nil {
		return fmt.Errorf("resolving model on %q: %w", providerName, err)
	}

	// Save state for rollback.
	prevProvider := a.agent.Provider
	prevModelName := a.agent.Model.Name
	prevModelProvider := a.agent.Model.Provider
	prevResolved := a.resolved.model
	prevThink := a.agent.Model.Think

	a.agent.Provider = newProvider
	a.applyNoop(a.agent)

	a.agent.Model.Name = modelName
	a.agent.Model.Provider = providerName
	a.resolved.model = &resolved

	// Coerce thinking to match new model capabilities.
	if !resolved.Capabilities.Thinking() {
		a.agent.Model.Think = thinking.NewValue(false)
	} else if !resolved.Capabilities.ThinkingLevels() && a.agent.Model.Think.IsString() {
		a.agent.Model.Think = thinking.NewValue(true)
	}

	if err := a.rebuildState(); err != nil {
		a.agent.Provider = prevProvider
		a.agent.Model.Name = prevModelName
		a.agent.Model.Provider = prevModelProvider
		a.resolved.model = prevResolved
		a.agent.Model.Think = prevThink

		return err
	}

	return nil
}

// SwitchMode switches to a different mode (preserves conversation).
// Empty string clears the mode (no mode prompt or mode-specific tools).
func (a *Assistant) SwitchMode(mode string) error {
	if mode != "" && a.cfg.Modes.Get(mode) == nil {
		return fmt.Errorf("mode %q not found", mode)
	}

	prev := a.agent.Mode

	a.agent.Mode = mode

	if err := a.rebuildState(); err != nil {
		a.agent.Mode = prev

		return err
	}

	return nil
}

// TemplateData builds a full TemplateData from the current assistant state.
// Used by rebuildState and feature resolve methods to avoid partial construction.
func (a *Assistant) TemplateData() config.TemplateData {
	// Compute sandbox display text.
	restrictions := a.cfg.EffectiveRestrictions()
	sandboxDisplay := ""

	if a.toggles.sandboxRequested {
		sb := &sandbox.Sandbox{WorkDir: a.effectiveWorkDir()}
		sb.AddReadOnly(restrictions.ReadOnly...)
		sb.AddReadWrite(restrictions.ReadWrite...)

		sandboxDisplay = sb.String(true)
	}

	// Compute read-before policy.
	rbPolicy := a.cfg.Features.ToolExecution.ReadBefore.ToPolicy()

	// Compute tool policy.
	r := a.resolved.config
	toolPolicy := a.cfg.EffectiveToolPolicy(r.Agent, r.Mode)

	return config.TemplateData{
		Config: config.ConfigPaths{
			Global:  a.paths.Global,
			Project: a.paths.Home,
			// Source is set per-agent in BuildAgent, not here.
		},
		LaunchDir: a.paths.Launch,
		WorkDir:   a.effectiveWorkDir(),
		Model:     config.NewModelData(a.resolved.model.Deref()),
		Provider:  r.Provider,
		Agent:     r.Agent,
		Mode:      config.ModeData{Name: r.Mode},
		Vars:      config.ToAnyMap(a.setVars),
		Sandbox:   config.NewSandboxData(a.toggles.sandbox, a.toggles.sandboxRequested, restrictions, sandboxDisplay),
		ReadBefore: config.ReadBeforeData{
			Write:  rbPolicy.Write,
			Delete: rbPolicy.Delete,
		},
		ToolPolicy: config.ToolPolicyData{
			Auto:    toolPolicy.Auto,
			Confirm: toolPolicy.Confirm,
			Deny:    toolPolicy.Deny,
			Display: toolPolicy.Display(),
		},
		Hooks: config.NewHooksData(a.cfg.FilteredHooks(a.agent.Name, a.agent.Mode)),
	}
}

// rebuildState rebuilds all config-derived state after a mutation.
// This is cheap and called from all state-changing methods to keep everything in sync.
// Returns an error if BuildAgent or hooks fail — callers must handle rollback.
//
// Must be called after changes to: agent, mode, model, thinking, sandbox,
// tool filters, loaded tools, MCP connections, or feature overrides.
//
// Rebuilds: effective features, opt-in tools, template data, agent prompt + tools,
// hooks, sandbox, tool policy, injectors, and emits a status update.
//
// Invalidates: compact model, title model, thinking model, guardrail models (×2).
// These are nil-cleared to force lazy re-resolution on next use.
//
// Called from 22 sites: SwitchAgent, setAgentFailover, SwitchModel, SwitchMode,
// ToggleThink, SetThink, CycleThink, ResizeContext, SetDone, EnableAsk,
// FilterTools, ToggleSandbox, MergeFeatures, SetSandbox, SetWorkDir,
// reloadConfig, reconnectMCP, CompactWith, SetReadBeforePolicy,
// RegisterMCPSession, ResumeSession, and recursively via LoadTools callback.
// RebuildState recomputes the agent's active tool set, features, prompt, and status.
func (a *Assistant) RebuildState() error {
	return a.rebuildState()
}

func (a *Assistant) rebuildState() error {
	// 1. Recompute effective features FIRST so BuildAgent reads correct state.
	effective, err := a.cfg.ResolveFeatures(a.globalFeatures, a.agent.Name, a.agent.Mode)
	if err != nil {
		return err
	}

	if a.tools.extraFeatures != nil {
		if err := effective.MergeFrom(*a.tools.extraFeatures); err != nil {
			return fmt.Errorf("merging extra features: %w", err)
		}
	}

	// CLI overrides (--max-steps, --token-budget, --override) are the final
	// config layer — applied after all merges. One call, one mechanism.
	if len(a.overrideNodes) > 0 {
		scratch := config.OverrideTarget{Features: effective}
		if err := a.overrideNodes.Apply(&scratch); err != nil {
			return fmt.Errorf("applying overrides: %w", err)
		}

		effective = scratch.Features
	}

	// Runtime toggles (/sandbox, /readbefore) are written to cfg.Features
	// AFTER this and still win over CLI flags.

	a.cfg.Features = effective

	// Recompute opt-in tools from resolved features + plugin flags.
	a.rt.OptInTools = slices.Clone(a.cfg.Features.ToolExecution.OptIn)
	if a.tools.plugins != nil {
		for _, pt := range a.tools.plugins.Tools() {
			if pt.IsOptIn() {
				a.rt.OptInTools = append(a.rt.OptInTools, pt.Name())
			}
		}
	}

	// sandboxEnabled is authoritative — not stored in any overlay.
	a.cfg.Features.Sandbox.Enabled = new(a.toggles.sandbox)
	// Read-before runtime override is authoritative — tracker holds the runtime value.
	if override := a.tracker.RuntimeOverride(); override != nil {
		a.cfg.Features.ToolExecution.ReadBefore.Write = new(override.Write)
		a.cfg.Features.ToolExecution.ReadBefore.Delete = new(override.Delete)
	}

	// 2. Template data (uses a.toggles.sandbox, a.cfg.EffectiveRestrictions).
	data := a.TemplateData()

	// 3. Manage Done/Ask in AllTools so they flow through the single Config.Tools() pipeline.
	a.rt.AllTools.Remove("Done")
	a.rt.AllTools.Remove("Ask")

	if a.toggles.doneActive {
		a.rt.AllTools.Add(done.New(func(_ string) { a.toggles.doneSignaled = true }))
	}

	if a.askCallback != nil {
		a.rt.AllTools.Add(ask.New(a.askCallback))
	}

	// 4. Sync task-level tool filter so BuildAgent/Tools renders {{ .Tools.Eager }}/{{ .Tools.Deferred }} correctly.
	a.rt.ExtraFilter = a.tools.extraFilter

	// 5. Sync loaded tools so Config.Tools() knows which deferred tools stay eager.
	a.rt.LoadedTools = a.tools.loaded

	// 6. Build condition state for tool filtering.
	m := a.resolved.model.Deref()

	a.rt.ConditionState = &condition.State{
		// Todo state.
		Todo: condition.TodoState{
			Empty: a.tools.todo.Len() == 0,
			Done: a.tools.todo.Len() > 0 && len(a.tools.todo.FindPending()) == 0 &&
				len(a.tools.todo.FindInProgress()) == 0,
			Pending: len(a.tools.todo.FindPending()) > 0 || len(a.tools.todo.FindInProgress()) > 0,
		},
		// Token/message metrics.
		Tokens: condition.TokensState{
			Percent: a.Status().Tokens.Percent,
			Total:   a.session.stats.Tokens.In + a.session.stats.Tokens.Out,
		},
		MessageCount: a.builder.Len(),
		// Stats.
		Tools: condition.ToolsState{
			Errors: a.session.stats.Tools.Errors,
			Calls:  a.session.stats.Tools.Calls,
		},
		Turns:       a.session.stats.Turns,
		Compactions: a.session.stats.Compactions,
		// Loop state.
		Iteration: a.loop.iteration,
		// Session mode.
		Auto: a.toggles.auto,
		// Model metadata.
		Model: condition.ModelState{
			Name:          m.Name,
			ParamCount:    int64(m.ParameterCount),
			ContextLength: int(m.ContextLength),
			Capabilities:  m.Capabilities.Map(),
		},
	}

	// 7. Build agent with CORRECT features.
	prompt, _, eager, deferred, err := a.cfg.BuildAgent(a.agent.Name, a.agent.Mode, a.agent.System, data, a.paths, a.rt)
	if err != nil {
		return fmt.Errorf("rebuilding agent state: %w", err)
	}

	// 8. Build hooks.
	hooksRunner, err := hooks.New(a.cfg.FilteredHooks(a.agent.Name, a.agent.Mode))
	if err != nil {
		return fmt.Errorf("loading hooks: %w", err)
	}

	// 9. All construction succeeded — swap atomically.
	a.agent.Prompt = prompt.String()
	a.agent.Tools = eager
	a.builder.UpdateSystemPrompt(a.ctx, a.agent.Prompt)

	a.resolved.schemaTokens = a.estimator.EstimateLocal(a.agent.Tools.Schemas().Render())

	a.resolved.hooks = hooksRunner
	a.resolved.hooks.OnStart = func(name string) {
		a.send(ui.SpinnerMessage{Text: name + " running…"})
	}

	// Invalidate feature model caches — the active agent's feature overrides
	// may point to different models than before. Nil-clearing forces
	// lazy re-resolution on next use. Agents are created on-demand via ResolveAgent.
	a.resolved.compactModel = nil
	a.resolved.titleModel = nil
	a.resolved.thinkingModel = nil
	a.resolved.guardrail.toolCalls.model = nil
	a.resolved.guardrail.userMessages.model = nil

	// Apply text overrides (Done/Ask now flow through Config.Tools() for filtering).
	a.cfg.ToolDefs.Apply(&a.agent.Tools)

	// Store deferred set for error messages (tools.go uses it to distinguish
	// "deferred but not loaded" from "disabled by mode").
	a.tools.deferred = deferred

	// Inject LoadTools meta-tool when deferred tools exist.
	// Created fresh each rebuild (Done tool pattern) — captures current deferred set.
	if len(deferred) > 0 {
		lt := loadtools.New(deferred, func(names []string) error {
			for _, n := range names {
				a.tools.loaded[n] = true
			}

			a.rt.LoadedTools = a.tools.loaded

			return a.rebuildState()
		})
		a.agent.Tools.Add(lt)
	}

	a.resolved.sandbox = buildSandbox(a.toggles.sandbox, a.cfg.EffectiveRestrictions(), a.effectiveWorkDir())
	a.resolved.toolPolicy = new(a.cfg.EffectiveToolPolicy(a.agent.Name, a.agent.Mode))
	a.rebuildInjectors()

	// Snapshot the effective runtime config — everything above has settled.
	a.resolved.config = config.Resolved{
		Agent:      a.agent.Name,
		Mode:       a.agent.Mode,
		Provider:   a.agent.Model.Provider,
		Model:      a.agent.Model.Name,
		Think:      a.agent.Model.Think,
		Context:    a.agent.Model.Context,
		Generation: a.agent.Model.Generation,
		Thinking:   a.agent.Thinking,
		Features:   a.cfg.Features,
		Sandbox:    a.toggles.sandbox,
		Verbose:    a.toggles.verbose,
		Auto:       a.toggles.auto,
		Done:       a.toggles.doneActive,
	}

	a.EmitAll()

	return nil
}

// NextAgent cycles to the next available agent.
func (a *Assistant) NextAgent() error {
	agents := a.cfg.Agents.Filter(config.Agent.Visible).Names()
	next := nextInSlice(a.agent.Name, agents)

	return a.SwitchAgent(next, "cycle")
}

// NextMode cycles to the next available mode.
func (a *Assistant) NextMode() error {
	modes := a.cfg.Modes.Filter(config.Mode.Visible).Names()
	next := nextInSlice(a.agent.Mode, modes)

	return a.SwitchMode(next)
}

func nextInSlice(current string, items []string) string {
	i := slices.IndexFunc(items, func(item string) bool {
		return strings.EqualFold(item, current)
	})

	if i < 0 {
		if len(items) > 0 {
			return items[0]
		}

		return current
	}

	return items[(i+1)%len(items)]
}

// ToggleThink toggles thinking false <-> true.
func (a *Assistant) ToggleThink() error {
	prev := a.agent.Model.Think

	if a.agent.Model.Think.Bool() {
		a.agent.Model.Think = thinking.NewValue(false)
	} else {
		a.agent.Model.Think = thinking.NewValue(true)
	}

	if err := a.rebuildState(); err != nil {
		a.agent.Model.Think = prev

		return err
	}

	return nil
}

// SetThink sets the thinking mode.
func (a *Assistant) SetThink(t thinking.Value) error {
	prev := a.agent.Model.Think

	a.agent.Model.Think = t

	if err := a.rebuildState(); err != nil {
		a.agent.Model.Think = prev

		return err
	}

	return nil
}

// CycleThink cycles through: false -> true -> low -> medium -> high -> false.
func (a *Assistant) CycleThink() error {
	prev := a.agent.Model.Think

	states := thinking.CycleStates()
	current := a.agent.Model.Think.Value
	idx := (slices.Index(states, current) + 1) % len(states)

	a.agent.Model.Think = thinking.NewValue(states[idx])

	if err := a.rebuildState(); err != nil {
		a.agent.Model.Think = prev

		return err
	}

	return nil
}

// ResizeContext sets the context window size.
func (a *Assistant) ResizeContext(size int) error {
	if size < 0 {
		return fmt.Errorf("context size must be non-negative, got %d", size)
	}

	prev := a.agent.Model.Context

	a.agent.Model.Context = size

	if err := a.rebuildState(); err != nil {
		a.agent.Model.Context = prev

		return err
	}

	return nil
}

// ToggleVerbose toggles the UI display of thinking content.
func (a *Assistant) ToggleVerbose() {
	a.toggles.verbose = !a.toggles.verbose
	a.resolved.config.Verbose = a.toggles.verbose
	a.EmitDisplayHints()
}

// SetAuto sets Auto mode to the given value.
func (a *Assistant) SetAuto(val bool) {
	a.toggles.auto = val
	a.resolved.config.Auto = val
	a.EmitDisplayHints()
}

// ToggleAuto toggles Auto mode on/off.
func (a *Assistant) ToggleAuto() {
	a.toggles.auto = !a.toggles.auto
	a.resolved.config.Auto = a.toggles.auto
	a.EmitDisplayHints()
}

// SetDone sets the Done tool to the given state.
// Delegates to rebuildState so the tool respects agent+mode filtering.
func (a *Assistant) SetDone(val bool) error {
	a.toggles.doneActive = val

	return a.rebuildState()
}


// EnableAsk registers the Ask tool callback.
// Delegates to rebuildState so the tool respects agent+mode filtering.
func (a *Assistant) EnableAsk(cb func(context.Context, ask.Request) (string, error)) error {
	a.askCallback = cb

	return a.rebuildState()
}

// FilterTools sets a persistent additional tool filter applied after agent+mode.
// Used by task runner to restrict tools beyond what agent+mode provide.
func (a *Assistant) FilterTools(filter tool.Filter) error {
	a.tools.extraFilter = &filter

	return a.rebuildState()
}

// ToggleSandbox toggles sandbox enforcement for tool execution.
func (a *Assistant) ToggleSandbox() error {
	if a.toggles.sandboxRequested {
		a.toggles.sandboxRequested = false
		a.toggles.sandbox = false
	} else {
		a.toggles.sandboxRequested = true
		a.toggles.sandbox = sandbox.IsAvailable()
	}

	return a.rebuildState()
}

// MergeFeatures applies feature overrides on top of the current effective features.
// Non-zero overlay fields win; zero/nil falls through.
// Routes through rebuildState so prompt, sandbox, hooks, and toolPolicy are rebuilt.
func (a *Assistant) MergeFeatures(overlay config.Features) error {
	a.tools.extraFeatures = &overlay

	return a.rebuildState()
}

// SetSandbox sets the sandbox state explicitly.
func (a *Assistant) SetSandbox(enabled bool) error {
	a.toggles.sandboxRequested = enabled

	if enabled {
		a.toggles.sandbox = sandbox.IsAvailable()
	} else {
		a.toggles.sandbox = false
	}

	return a.rebuildState()
}

// SandboxDisplay returns a human-readable summary of sandbox state and restrictions.
func (a *Assistant) SandboxDisplay() string {
	if !a.toggles.sandboxRequested {
		return "Sandbox: disabled"
	}

	restrictions := a.cfg.EffectiveRestrictions()

	sb := &sandbox.Sandbox{WorkDir: a.effectiveWorkDir()}
	sb.AddReadOnly(restrictions.ReadOnly...)
	sb.AddReadWrite(restrictions.ReadWrite...)

	if !sb.HasRules() {
		if a.toggles.sandbox {
			return "Sandbox: enabled (no restrictions configured)"
		}

		return "Sandbox: advisory (Landlock not available, no restrictions configured)"
	}

	if !a.toggles.sandbox {
		return "Sandbox: advisory (Landlock not available)\n\n" + sb.String()
	}

	return "Sandbox: enabled\n\n" + sb.String()
}

// SandboxEnabled returns current sandbox enforcement state.
func (a *Assistant) SandboxEnabled() bool {
	return a.toggles.sandbox || a.resolved.sandbox != nil
}

// SandboxRequested returns whether the user wants sandbox, independent of kernel support.
func (a *Assistant) SandboxRequested() bool {
	return a.toggles.sandboxRequested
}
