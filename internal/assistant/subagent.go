package assistant

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/hooks"
	"github.com/idelchi/aura/internal/subagent"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/tokens"
)

// RunSubagent spawns a one-shot subagent with isolated context and returns its result.
// When agentName is empty it resolves to Features.Subagent.DefaultAgent, then falls
// back to the parent's agent name. A fresh agent is always created via agent.New()
// so parallel Task calls never share provider or tool state.
func (a *Assistant) RunSubagent(ctx context.Context, agentName, prompt string) (subagent.Result, error) {
	if agentName == "" {
		agentName = a.cfg.Features.Subagent.DefaultAgent
	}

	if agentName == "" {
		agentName = a.agent.Name
	}

	debug.Log("[subagent] spawning agent=%q promptLen=%d", agentName, len(prompt))

	// Reset features to global base so child agent's tools/prompt are built from
	// global → child (not global → parent → child).
	childCfg := a.cfg

	childCfg.Features = a.globalFeatures

	child, err := agent.New(childCfg, a.paths, a.rt, agentName)
	if err != nil {
		return subagent.Result{}, fmt.Errorf("resolve subagent %q: %w", agentName, err)
	}

	// Resolve child's effective features for Runner fields.
	childFeatures, err := a.cfg.ResolveFeatures(a.globalFeatures, agentName, child.Mode)
	if err != nil {
		return subagent.Result{}, fmt.Errorf("resolve subagent features: %w", err)
	}

	// Resolve model metadata via provider API (one-shot, no caching needed).
	resolved, err := child.Provider.Model(ctx, child.Model.Name)
	if err != nil {
		return subagent.Result{}, fmt.Errorf("resolve subagent model: %w", err)
	}

	// Config context overrides resolved context (same as main assistant's chat()).
	contextLength := int(resolved.ContextLength)

	if child.Model.Context > 0 {
		contextLength = child.Model.Context
	}

	// Strip Task (recursion) and Ask (no user interaction) from subagent tool set.
	tools := child.Tools.Filtered(nil, []string{"Task", "Ask"})

	// Resolve hooks for the child agent.
	childHooks := a.cfg.FilteredHooks(child.Name, child.Mode)

	hooksRunner, err := hooks.New(childHooks)
	if err != nil {
		return subagent.Result{}, fmt.Errorf("subagent hooks: %w", err)
	}

	hooksRunner.OnStart = func(name string) {
		a.send(ui.SpinnerMessage{Text: name + " running…"})
	}

	// Resolve child's read-before policy from its config.
	childPolicy := childFeatures.ToolExecution.ReadBefore.ToPolicy()

	// /readbefore runtime change overrides everything (session-wide decision).
	if override := a.tracker.RuntimeOverride(); override != nil {
		childPolicy = *override
	}

	runner := subagent.Runner{
		Provider:         child.Provider,
		Tools:            tools,
		Prompt:           child.Prompt,
		Model:            resolved,
		Think:            child.Model.Think.Ptr(),
		ContextLength:    contextLength,
		Events:           nil, // subagent builder events are not rendered in TUI
		MaxSteps:         childFeatures.Subagent.MaxSteps,
		ExecuteOverride:  a.subagentExecuteOverride(),
		PathChecker:      a.PathChecker(),
		ResultGuard:      buildResultGuard(childFeatures, a.estimator),
		HooksRunner:      hooksRunner,
		LSPManager:       a.tools.lsp,
		CWD:              a.effectiveWorkDir(),
		Estimate:         a.estimator.EstimateLocal,
		Features:         childFeatures,
		ReadBeforePolicy: childPolicy,
	}

	result, err := runner.Run(ctx, prompt)
	if err != nil {
		return result, err
	}

	// Propagate subagent token usage and tool calls to session stats.
	a.session.stats.RecordTokens(result.Usage.Input, result.Usage.Output)
	a.session.stats.MergeToolCalls(result.Tools)

	debug.Log("[subagent] done agent=%q calls=%d tokens=%d", agentName, result.ToolCalls, result.Usage.Total())

	return result, nil
}

// subagentExecuteOverride returns executeSandboxed if sandbox is enabled, nil otherwise.
func (a *Assistant) subagentExecuteOverride() func(context.Context, string, map[string]any) (string, error) {
	if !a.toggles.sandbox {
		return nil
	}

	return a.executeSandboxed
}

// PathChecker wraps CheckPaths for the Runner's PathChecker signature.
func (a *Assistant) PathChecker() subagent.PathChecker {
	if a.resolved.sandbox == nil {
		return nil
	}

	return func(read, write []string) error {
		return a.CheckPaths(read, write)
	}
}

// buildResultGuard returns a guard that rejects tool results exceeding the absolute token limit.
// Uses tokens mode (not percentage) because the subagent has its own context window — percentage
// checks against the main assistant's context state would produce wrong results.
func buildResultGuard(features config.Features, est *tokens.Estimator) subagent.ResultGuardFunc {
	maxTokens := features.ToolExecution.Result.MaxTokens

	return func(ctx context.Context, _, result string) error {
		t := est.Estimate(ctx, result)
		if maxTokens > 0 && t > maxTokens {
			return fmt.Errorf("tool result too large (%d estimated tokens, limit %d)", t, maxTokens)
		}

		return nil
	}
}
