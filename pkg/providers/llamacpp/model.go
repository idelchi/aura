package llamacpp

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/pkg/llm/model"
)

// Model fetches metadata for the specified model.
func (c *Client) Model(ctx context.Context, name string) (model.Model, error) {
	info, err := c.Show(ctx, name)
	if err != nil {
		return model.Model{}, fmt.Errorf("fetching model info for %q: %w", name, err)
	}

	m := model.Model{
		Name:           name,
		ParameterCount: model.ParseParameterName(name),
		ContextLength:  model.ContextLength(info.DefaultGenerationSettings.ContextLength),
	}

	return WithCapabilities(m, info), nil
}
