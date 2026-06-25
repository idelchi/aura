package google

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/thinking"
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

	opts, err := buildProviderOptions(req)
	if err != nil {
		return message.Message{}, usage.Usage{}, err
	}

	call.ProviderOptions = opts

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
func buildProviderOptions(req request.Request) (fantasy.ProviderOptions, error) {
	if req.Think == nil || req.Think.IsUnset() || req.Think.IsOff() {
		return fantasy.ProviderOptions{}, nil
	}

	tc := &fantasygoogle.ThinkingConfig{
		IncludeThoughts: fantasy.Opt(true),
	}

	// Only set ThinkingLevel for explicit effort values. Boolean true means
	// IncludeThoughts only, because not all Gemini models support levels.
	if effort, ok := req.Think.Effort(); ok {
		level, set, err := googleThinkingLevel(effort)
		if err != nil {
			return nil, err
		}

		if !set {
			return fantasy.ProviderOptions{}, nil
		}

		tc.ThinkingLevel = &level
	}

	if req.Generation != nil && req.Generation.ThinkBudget != nil {
		budget := int64(*req.Generation.ThinkBudget)

		tc.ThinkingBudget = &budget
	}

	return fantasy.ProviderOptions{fantasygoogle.Name: &fantasygoogle.ProviderOptions{
		ThinkingConfig: tc,
	}}, nil
}

func googleThinkingLevel(effort thinking.Effort) (string, bool, error) {
	switch effort {
	case thinking.None:
		return "", false, nil
	case thinking.Minimal:
		return fantasygoogle.ThinkingLevelMinimal, true, nil
	case thinking.Low:
		return fantasygoogle.ThinkingLevelLow, true, nil
	case thinking.Medium:
		return fantasygoogle.ThinkingLevelMedium, true, nil
	case thinking.High:
		return fantasygoogle.ThinkingLevelHigh, true, nil
	case thinking.XHigh, thinking.Max:
		return "", false, fmt.Errorf("google thinking effort %q is not supported", effort)
	default:
		return "", false, fmt.Errorf("google thinking effort %q is not supported", effort)
	}
}
