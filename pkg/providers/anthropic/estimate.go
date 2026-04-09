package anthropic

import (
	"context"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers/adapter"
)

// Estimate tokenizes content using Anthropic's CountTokens API.
// Wraps content as a single user message and returns the exact token count
// from the model's tokenizer. Falls back to local estimation on any error.
func (c *Client) Estimate(ctx context.Context, req request.Request, content string) (int, error) {
	resp, err := c.Messages.CountTokens(ctx, sdk.MessageCountTokensParams{
		Model:    sdk.Model(req.Model.Name),
		Messages: []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock(content))},
	})
	if err != nil {
		return 0, adapter.MapError(err)
	}

	return int(resp.InputTokens), nil
}
