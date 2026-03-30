package anthropic

import (
	"context"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
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

	call.ProviderOptions = buildProviderOptions(req)

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
func buildProviderOptions(req request.Request) fantasy.ProviderOptions {
	if req.Think == nil || !req.Think.Bool() {
		return nil
	}

	opts := &fanthropic.ProviderOptions{}

	if req.Generation != nil && req.Generation.ThinkBudget != nil {
		opts.Thinking = &fanthropic.ThinkingProviderOption{
			BudgetTokens: int64(*req.Generation.ThinkBudget),
		}
	} else {
		effort := fanthropic.Effort(req.Think.String())

		opts.Effort = &effort
	}

	return fantasy.ProviderOptions{fanthropic.Name: opts}
}
