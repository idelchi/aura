package google

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/llm/model"
)

// Model returns metadata for a specific model from the Google Gemini API.
func (c *Client) Model(ctx context.Context, name string) (model.Model, error) {
	m, err := c.Client.Models.Get(ctx, name, nil)
	if err != nil {
		return model.Model{}, fmt.Errorf("fetching model %q: %w", name, err)
	}

	cleanName := strings.TrimPrefix(m.Name, "models/")

	mdl := model.Model{
		Name:           cleanName,
		ParameterCount: model.ParseParameterName(cleanName),
		ContextLength:  model.ContextLength(m.InputTokenLimit),
	}

	enrichFromAPI(&mdl, m.Thinking, m.SupportedActions)

	return mdl, nil
}
