package copilot

import (
	"context"
	"fmt"
	"slices"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// Chat routes the request to the appropriate underlying provider based on the model's supported endpoints.
func (c *Client) Chat(ctx context.Context, req request.Request, fn stream.Func) (message.Message, usage.Usage, error) {
	if err := c.ensureModels(ctx); err != nil {
		return message.Message{}, usage.Usage{}, err
	}

	c.mu.Lock()
	info, ok := c.models[req.Model.Name]
	c.mu.Unlock()

	if !ok {
		return message.Message{}, usage.Usage{}, fmt.Errorf("model %q not found in copilot model list", req.Model.Name)
	}

	if slices.Contains(info.Endpoints, "/v1/messages") {
		return c.anthropic.Chat(ctx, req, fn)
	}

	if slices.Contains(info.Endpoints, "/responses") {
		return c.openai.Chat(ctx, req, fn)
	}

	return message.Message{}, usage.Usage{}, fmt.Errorf("model %q has no supported endpoint", req.Model.Name)
}
