package openai

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/pkg/llm/model"
)

// Model resolves an exact model ID from the provider's model catalog.
func (c *Client) Model(ctx context.Context, name string) (model.Model, error) {
	models, err := c.Models(ctx)
	if err != nil {
		return model.Model{}, err
	}

	if !models.Exists(name) {
		return model.Model{}, fmt.Errorf("model %q not found", name)
	}

	return models.Get(name), nil
}
