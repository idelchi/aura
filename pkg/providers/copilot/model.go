package copilot

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/pkg/llm/model"
)

// Model returns metadata for a specific model by name.
func (c *Client) Model(ctx context.Context, name string) (model.Model, error) {
	if err := c.ensureModels(ctx); err != nil {
		return model.Model{}, err
	}

	c.mu.Lock()
	info, ok := c.models[name]
	c.mu.Unlock()

	if !ok {
		return model.Model{}, fmt.Errorf("model %q not found in copilot model list", name)
	}

	return info.Model, nil
}
