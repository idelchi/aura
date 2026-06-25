package anthropic

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers/adapter"

	"charm.land/fantasy"
	fanthropic "charm.land/fantasy/providers/anthropic"
)

// Chat sends a streaming chat completion request to the Anthropic API.
func (c *Client) Chat(
	ctx context.Context,
	req request.Request,
	streamFunc stream.Func,
) (message.Message, usage.Usage, error) {
	if req.Think != nil && req.Think.Bool() && req.Generation != nil && req.Generation.Temperature != nil {
		debug.Log("[anthropic] thinking enabled — ignoring temperature=%.2f (requires 1.0)",
			*req.Generation.Temperature)
	}

	lm, err := c.Fantasy.LanguageModel(ctx, req.Model.Name)
	if err != nil {
		return message.Message{}, usage.Usage{}, adapter.MapError(err)
	}

	call := adapter.ToCall(req.Messages, req.Tools)

	// Thinking disables temperature — don't set it via SetGeneration.
	if req.Think == nil || !req.Think.Bool() {
		adapter.SetGeneration(&call, req.Generation)
	} else if req.Generation != nil {
		// Apply all generation params except temperature when thinking is enabled.
		gen := *req.Generation

		gen.Temperature = nil
		adapter.SetGeneration(&call, &gen)
	}

	opts, err := buildProviderOptions(req)
	if err != nil {
		return message.Message{}, usage.Usage{}, err
	}

	call.ProviderOptions = opts

	iter, err := lm.Stream(ctx, call)
	if err != nil {
		return message.Message{}, usage.Usage{}, adapter.MapError(err)
	}

	msg, u, err := adapter.StreamToMessage(iter, streamFunc, fanthropic.Name)
	if err != nil {
		return msg, u, adapter.MapError(err)
	}

	return msg, u, nil
}

// buildProviderOptions sets Anthropic-specific thinking options.
func buildProviderOptions(req request.Request) (fantasy.ProviderOptions, error) {
	if req.Think == nil || req.Think.IsUnset() || req.Think.IsOff() {
		return fantasy.ProviderOptions{}, nil
	}

	opts := &fanthropic.ProviderOptions{}

	if req.Generation != nil && req.Generation.ThinkBudget != nil {
		opts.Thinking = &fanthropic.ThinkingProviderOption{
			BudgetTokens: int64(*req.Generation.ThinkBudget),
		}
	} else {
		effort, ok, err := anthropicEffort(req.Think)
		if err != nil {
			return nil, err
		}

		if !ok {
			return fantasy.ProviderOptions{}, nil
		}

		opts.Effort = &effort
	}

	return fantasy.ProviderOptions{fanthropic.Name: opts}, nil
}

func anthropicEffort(value *thinking.Value) (fanthropic.Effort, bool, error) {
	if value == nil || value.IsAuto() {
		return fanthropic.EffortHigh, true, nil
	}

	effort, ok := value.Effort()
	if !ok {
		return fanthropic.EffortHigh, true, nil
	}

	switch effort {
	case thinking.None:
		return "", false, nil
	case thinking.Low, thinking.Medium, thinking.High, thinking.XHigh, thinking.Max:
		return fanthropic.Effort(effort), true, nil
	case thinking.Minimal:
		return "", false, fmt.Errorf("anthropic thinking effort %q is not supported", effort)
	default:
		return "", false, fmt.Errorf("anthropic thinking effort %q is not supported", effort)
	}
}
