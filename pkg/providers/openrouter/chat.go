package openrouter

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers/adapter"

	"charm.land/fantasy"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
)

// Chat sends a chat completion request via Fantasy with streaming support.
func (c *Client) Chat(
	ctx context.Context,
	req request.Request,
	streamFunc stream.Func,
) (message.Message, usage.Usage, error) {
	lm, err := c.fantasy.LanguageModel(ctx, req.Model.Name)
	if err != nil {
		return message.Message{}, usage.Usage{}, adapter.MapError(err)
	}

	call := adapter.ToCall(req.Messages, req.Tools)
	adapter.SetGeneration(&call, req.Generation)

	call.ProviderOptions = buildProviderOptions(req)

	iter, err := lm.Stream(ctx, call)
	if err != nil {
		return message.Message{}, usage.Usage{}, adapter.MapError(err)
	}

	msg, u, err := adapter.StreamToMessage(iter, streamFunc, fantasyopenrouter.Name)
	if err != nil {
		return msg, u, adapter.MapError(err)
	}

	return msg, u, nil
}

// buildProviderOptions sets OpenRouter-specific options (reasoning).
func buildProviderOptions(req request.Request) fantasy.ProviderOptions {
	if req.Think == nil || !req.Think.Bool() {
		return nil
	}

	opts := &fantasyopenrouter.ProviderOptions{}

	reasoning := &fantasyopenrouter.ReasoningOptions{
		Enabled: fantasy.Opt(true),
	}

	if req.Generation != nil && req.Generation.ThinkBudget != nil {
		reasoning.MaxTokens = fantasy.Opt(int64(*req.Generation.ThinkBudget))
	} else {
		effort := fantasyopenrouter.ReasoningEffort(req.Think.String())

		reasoning.Effort = &effort
	}

	opts.Reasoning = reasoning

	return fantasy.ProviderOptions{fantasyopenrouter.Name: opts}
}
