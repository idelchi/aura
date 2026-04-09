package assistant

import (
	"context"
	"errors"
	"fmt"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// manageThinking applies per-agent thinking management to conversation history.
// Mutates the builder in place. Returns error on failure (no silent degradation).
func (a *Assistant) manageThinking(ctx context.Context) error {
	cfg := a.cfg.Features.Thinking

	debug.Log("[thinking] manageThinking: mode=%q keepLast=%d tokenThreshold=%d",
		a.agent.Thinking, cfg.KeepLast, cfg.TokenThreshold)

	switch a.agent.Thinking {
	case thinking.Strip:
		a.StripThinking(ctx, cfg.KeepLast, cfg.TokenThreshold)

		return nil
	case thinking.Rewrite:
		// Rewrite rewrites thinking text, which invalidates ThinkingSignature.
		// Providers that require signatures (Anthropic, Google) will reject the mismatch.
		for _, msg := range a.builder.History() {
			if msg.ThinkingSignature != "" {
				return errors.New("thinking: rewrite is incompatible with this provider — " +
					"ThinkingSignature cannot survive text modification; use thinking: strip instead",
				)
			}
		}

		return a.RewriteThinking(ctx, cfg.KeepLast, cfg.TokenThreshold)
	default:
		return nil
	}
}

// StripThinking clears .Thinking on messages older than keepLast that exceed tokenThreshold.
// Mutates the builder directly. No LLM call.
func (a *Assistant) StripThinking(ctx context.Context, keepLast, tokenThreshold int) {
	history := a.builder.History()
	boundary := len(history) - keepLast

	if boundary <= 0 {
		return
	}

	for i := range boundary {
		if history[i].Thinking == "" {
			continue
		}

		if a.estimator.Estimate(ctx, history[i].Thinking) < tokenThreshold {
			continue
		}

		debug.Log("[thinking] stripping index=%d", i)
		a.builder.UpdateThinking(i, "")
	}
}

// RewriteThinking finds the single message that just crossed the keepLast boundary
// and has a thinking block exceeding tokenThreshold, then rewrites it via one LLM call.
// Mutates the builder directly. At most one block qualifies per turn.
func (a *Assistant) RewriteThinking(ctx context.Context, keepLast, tokenThreshold int) error {
	history := a.builder.History()
	boundary := len(history) - keepLast

	if boundary <= 0 {
		return nil
	}

	// The block that just crossed the boundary is at index (boundary - 1).
	idx := boundary - 1

	original := history[idx].Thinking
	if original == "" {
		return nil
	}

	if a.estimator.Estimate(ctx, original) < tokenThreshold {
		return nil
	}

	debug.Log("[thinking] rewriting index=%d", idx)

	cfg := a.cfg.Features.Thinking

	resolved, err := a.ResolveAgent(ctx, FeatureAgentConfig{
		label:      "thinking",
		promptName: cfg.Prompt,
		agentName:  cfg.Agent,
		modelCache: &a.resolved.thinkingModel,
	})
	if err != nil {
		return fmt.Errorf("resolving thinking agent: %w", err)
	}

	summary, err := a.CallRewrite(ctx, resolved, original)
	if err != nil {
		return fmt.Errorf("thinking rewrite: %w", err)
	}

	a.builder.UpdateThinking(idx, summary)

	debug.Log("[thinking] rewritten index=%d original_len=%d rewrite_len=%d",
		idx, len(original), len(summary))

	return nil
}

// CallRewrite makes a single LLM call to rewrite one thinking block.
func (a *Assistant) CallRewrite(
	ctx context.Context,
	resolved FeatureResolution,
	thinkingContent string,
) (string, error) {
	a.send(ui.SpinnerMessage{Text: "Rewriting thinking..."})
	defer a.send(ui.SpinnerMessage{})

	req := request.Request{
		Model: resolved.mdl,
		Messages: message.New(
			message.Message{Role: roles.System, Content: resolved.prompt},
			message.Message{Role: roles.User, Content: thinkingContent},
		),
		ContextLength: resolved.contextLen,
		Truncate:      true,
		Shift:         true,
	}

	response, _, err := resolved.provider.Chat(ctx, req, noopStream)
	if err != nil {
		return "", fmt.Errorf("thinking chat: %w", err)
	}

	return response.Content, nil
}
