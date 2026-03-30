package openrouter

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/pkg/llm/model"
)

func (c *Client) ensureModels(ctx context.Context) error {
	c.mu.Lock()
	cached := c.cachedModels
	c.mu.Unlock()

	if cached != nil {
		return nil
	}

	models, err := c.Models(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.cachedModels = models
	c.mu.Unlock()

	return nil
}

// Model returns metadata for the specified model by name.
// Uses a lazy-loaded cache to avoid re-fetching the full model list on every call.
func (c *Client) Model(ctx context.Context, name string) (model.Model, error) {
	if err := c.ensureModels(ctx); err != nil {
		return model.Model{}, err
	}

	c.mu.Lock()
	cached := c.cachedModels
	c.mu.Unlock()

	if !cached.Exists(name) {
		return model.Model{}, fmt.Errorf("model %q not found", name)
	}

	return cached.Get(name), nil
}
