package openai

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/model"
)

// Models fetches all available models from the OpenAI-compatible API.
func (c *Client) Models(ctx context.Context) (model.Models, error) {
	models, err := c.List(ctx)
	if err != nil {
		return nil, err
	}

	return models, nil
}
