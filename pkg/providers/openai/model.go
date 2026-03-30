package openai

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/registry"
)

// Model fetches metadata for the specified model.
func (c *Client) Model(ctx context.Context, name string) (model.Model, error) {
	info, err := c.Client.Models.Get(ctx, name)
	if err != nil {
		return model.Model{}, fmt.Errorf("fetching model info for %q: %w", name, err)
	}

	// SDK returns only model ID — no context length, vision, or other capabilities.
	// ParameterCount is parsed from the name; everything else comes from registry.Enrich().
	m := model.Model{
		Name:           info.ID,
		ParameterCount: model.ParseParameterName(info.ID),
	}

	registry.Enrich("openai", &m)

	return m, nil
}
