package openai

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers/adapter"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

// Chat sends a streaming request via Fantasy using the Responses API.
func (c *Client) Chat(
	ctx context.Context,
	req request.Request,
	streamFunc stream.Func,
) (message.Message, usage.Usage, error) {
	lm, err := c.Fantasy.LanguageModel(ctx, req.Model.Name)
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

	msg, u, err := adapter.StreamToMessage(iter, streamFunc, fantasyopenai.Name)
	if err != nil {
		return msg, u, adapter.MapError(err)
	}

	return msg, u, nil
}

// buildProviderOptions sets OpenAI-specific options (reasoning effort, store).
// Uses *ProviderOptions which works for both Chat Completions and Responses API paths.
// Fantasy selects the API path based on model name (IsResponsesModel).
func buildProviderOptions(req request.Request) fantasy.ProviderOptions {
	opts := &fantasyopenai.ProviderOptions{}

	hasOpts := false

	if req.Think != nil && req.Think.Bool() {
		effort := fantasyopenai.ReasoningEffort(req.Think.String())

		opts.ReasoningEffort = &effort
		hasOpts = true
	}

	if req.Store != nil {
		opts.Store = req.Store
		hasOpts = true
	}

	if !hasOpts {
		return nil
	}

	return fantasy.ProviderOptions{fantasyopenai.Name: opts}
}
