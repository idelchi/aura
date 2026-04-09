package google

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers/adapter"

	"charm.land/fantasy"
	fantasygoogle "charm.land/fantasy/providers/google"
)

// Chat sends a streaming chat completion request to the Google Gemini API via Fantasy.
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

	msg, u, err := adapter.StreamToMessage(iter, streamFunc, fantasygoogle.Name)
	if err != nil {
		return msg, u, adapter.MapError(err)
	}

	return msg, u, nil
}

// buildProviderOptions sets Google-specific thinking options.
func buildProviderOptions(req request.Request) fantasy.ProviderOptions {
	if req.Think == nil || !req.Think.Bool() {
		return nil
	}

	tc := &fantasygoogle.ThinkingConfig{
		IncludeThoughts: fantasy.Opt(true),
	}

	// Only set ThinkingLevel for explicit string levels ("low"/"medium"/"high").
	// Boolean true → IncludeThoughts only (not all models support ThinkingLevel).
	// think.String() returns "medium" for bool true, so we must check IsBool() first.
	if !req.Think.IsBool() {
		switch req.Think.String() {
		case "low":
			level := fantasygoogle.ThinkingLevelLow

			tc.ThinkingLevel = &level
		case "medium":
			level := fantasygoogle.ThinkingLevelMedium

			tc.ThinkingLevel = &level
		case "high":
			level := fantasygoogle.ThinkingLevelHigh

			tc.ThinkingLevel = &level
		}
	}

	if req.Generation != nil && req.Generation.ThinkBudget != nil {
		budget := int64(*req.Generation.ThinkBudget)

		tc.ThinkingBudget = &budget
	}

	return fantasy.ProviderOptions{fantasygoogle.Name: &fantasygoogle.ProviderOptions{
		ThinkingConfig: tc,
	}}
}
