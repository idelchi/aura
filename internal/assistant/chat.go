package assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/truncate"
	"github.com/idelchi/aura/sdk"
)

// chat sends the current message history to the provider and returns the response.
func (a *Assistant) chat(ctx context.Context) (message.Message, error) {
	if _, err := a.ResolveModel(ctx, "chat", a.agent, &a.resolved.model); err != nil {
		return message.Message{}, err
	}

	// Error when context length is unknown and no absolute fallback is configured.
	// Without either, compaction cannot function and the session will eventually hit the
	// provider's hard context limit with no recovery. Skip for noop provider (returns zero
	// context by design — safe for --dry=noop).
	if a.noopProvider == nil && a.ContextLength() == 0 && a.cfg.Features.Compaction.MaxTokens == 0 {
		return message.Message{}, fmt.Errorf(
			"model %q reports no context length and no compaction.max_tokens configured — "+
				"set context: in your agent config or compaction.max_tokens in features",
			a.resolved.model.Deref().Name)
	}

	a.WireEstimation()

	if a.agent.Thinking != thinking.Keep {
		debug.Log("[chat] manageThinking mode=%q", a.agent.Thinking)

		if err := a.manageThinking(ctx); err != nil {
			debug.Log("[chat] manageThinking error: %v", err)

			return message.Message{}, fmt.Errorf("manage thinking: %w", err)
		}
	}

	history := a.builder.History().ForLLM()

	// Apply one-turn system prompt appendages from BeforeChat plugins.
	if len(a.loop.appendSystem) > 0 && len(history) > 0 && history[0].IsSystem() {
		extra := strings.Join(a.loop.appendSystem, "\n\n")

		history[0].Content = history[0].Content + "\n\n" + extra

		history[0].Tokens.Total += a.estimator.Estimate(ctx, extra)

		a.loop.appendSystem = nil
	}

	agentTools := a.agent.Tools
	if a.loop.toolsFilter != nil {
		agentTools = agentTools.Filtered(a.loop.toolsFilter.Enabled, a.loop.toolsFilter.Disabled)
	}

	// TransformMessages hook pipeline: let plugins modify the message array before the LLM call.
	// Ephemeral — builder history is untouched. Plugins re-apply all transforms every turn.
	if a.tools.injectors.HasTransformers() {
		sdkMsgs := toSDKMessages(history)
		state := a.InjectorState()
		transformed := a.tools.injectors.RunTransformMessages(ctx, state, sdkMsgs)

		if err := sdk.ValidateTransformed(transformed); err != nil {
			debug.Log("[chat] transform validation failed: %v, using original", err)
		} else {
			history = applyTransform(history, transformed)

			debug.Log("[chat] TransformMessages applied: %d → %d messages", len(sdkMsgs), len(transformed))
		}
	}

	tools := agentTools.Schemas()

	req := request.Request{
		Model:          a.resolved.model.Deref(),
		Think:          a.agent.Model.Think.Ptr(),
		Messages:       history,
		ContextLength:  a.agent.Model.Context,
		Tools:          tools,
		Generation:     a.agent.Model.Generation,
		ResponseFormat: a.agent.ResponseFormat,
	}

	lastRole := "none"

	if len(history) > 0 {
		lastRole = string(history[len(history)-1].Role)
	}

	debug.Log(
		"[chat] model=%s messages=%d tools=%d toolsFilter=%v lastRole=%s contextLength=%d resolvedContextLength=%d",
		a.resolved.model.Deref().Name,
		len(history),
		len(req.Tools),
		a.loop.toolsFilter,
		lastRole,
		a.agent.Model.Context,
		int(a.resolved.model.Deref().ContextLength),
	)

	streamFunc := func(thinking, content string, done bool) error {
		a.loop.streamStarted = true

		if thinking != "" {
			a.builder.AppendThinking(thinking)
		}

		if content != "" {
			a.builder.AppendContent(content)
		}

		return nil
	}

	previousInputTokens := a.tokens.lastInput

	response, usageData, err := a.agent.Provider.Chat(ctx, req, streamFunc)
	if err != nil {
		debug.Log("[chat] error: %v", err)

		return message.Message{}, err
	}

	debug.Log(
		"[chat] response content=%q thinking_len=%d tool_calls=%d usage_in=%d usage_out=%d",
		truncate.Truncate(
			response.Content,
			200,
		),
		len(response.Thinking),
		len(response.Calls),
		usageData.Input,
		usageData.Output,
	)

	// Backfill tool result tokens from API-reported delta before updating tracking state.
	// Delta = what the API charged for messages added since last chat() call.
	// Uses lastAPIInput (raw API value) instead of lastInputTokens (which may be
	// overwritten by EstimateTokens() between chat calls for display/compaction).
	// Includes tool results + any synthetics; synthetic overcount is the safe direction.
	if a.tokens.lastAPIInput > 0 {
		delta := usageData.Input - a.tokens.lastAPIInput - a.tokens.lastOutput
		if delta > 0 {
			a.builder.BackfillToolTokens(delta)
			debug.Log("[chat] backfilled tool tokens: delta=%d", delta)
		}
	}

	// Set exact output tokens on the assistant response with breakdown.
	// Uses provider tokenizer (estimateTokens) for content/thinking split.
	// Tools is the exact remainder — avoids estimating structured JSON.
	contentTokens := a.estimator.Estimate(ctx, response.Content)
	thinkingTokens := a.estimator.Estimate(ctx, response.Thinking)
	toolsTokens := max(usageData.Output-contentTokens-thinkingTokens, 0)

	response.Tokens = message.Tokens{
		Total:    usageData.Output,
		Content:  contentTokens,
		Thinking: thinkingTokens,
		Tools:    toolsTokens,
	}

	debug.Log("[chat] response tokens: %s", response.Tokens)

	// Accumulate token usage
	a.session.usage.Add(usageData)

	a.tokens.lastInput = usageData.Input
	a.tokens.lastOutput = usageData.Output
	a.tokens.lastAPIInput = usageData.Input

	// When tools are temporarily disabled by an injector, the API-reported input
	// tokens exclude tool schemas — deflating the count. Preserve the previous
	// accurate baseline and add only what the LLM just generated.
	// When previousInputTokens == 0 (after Compact() or first turn), keep the
	// API value — a deflated input is a much smaller error than 0 + Output.
	if a.loop.toolsFilter != nil && previousInputTokens > 0 {
		a.tokens.lastInput = previousInputTokens + usageData.Output
	}

	a.session.stats.RecordTokens(usageData.Input, usageData.Output)

	return response, nil
}
