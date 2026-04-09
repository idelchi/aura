package codex

import (
	"context"
	"strings"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers/adapter"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

// Chat injects store: false and instructions before delegating to Fantasy.
// Codex backend requires the instructions field explicitly (unlike standard OpenAI
// which accepts system messages in the input array).
func (c *Client) Chat(ctx context.Context, req request.Request, fn stream.Func) (message.Message, usage.Usage, error) {
	req.Store = new(false)

	lm, err := c.inner.Fantasy.LanguageModel(ctx, req.Model.Name)
	if err != nil {
		return message.Message{}, usage.Usage{}, adapter.MapError(err)
	}

	call := adapter.ToCall(req.Messages, req.Tools)
	adapter.SetGeneration(&call, req.Generation)

	// Codex backend requires instructions field — extract system messages.
	opts := &fantasyopenai.ResponsesProviderOptions{
		Store: new(false),
	}

	var instructions []string

	for _, msg := range req.Messages {
		if msg.Role == roles.System && msg.Content != "" {
			instructions = append(instructions, msg.Content)
		}
	}

	if len(instructions) > 0 {
		joined := strings.Join(instructions, "\n")

		opts.Instructions = &joined
	}

	if req.Think != nil && req.Think.Bool() {
		effort := fantasyopenai.ReasoningEffort(req.Think.String())

		opts.ReasoningEffort = &effort
	}

	call.ProviderOptions = fantasy.ProviderOptions{fantasyopenai.Name: opts}

	iter, err := lm.Stream(ctx, call)
	if err != nil {
		return message.Message{}, usage.Usage{}, adapter.MapError(err)
	}

	msg, u, err := adapter.StreamToMessage(iter, fn, fantasyopenai.Name)
	if err != nil {
		return msg, u, adapter.MapError(err)
	}

	return msg, u, nil
}
