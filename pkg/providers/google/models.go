package google

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/llm/model"
)

// Models returns all available models from the Google Gemini API.
func (c *Client) Models(ctx context.Context) (model.Models, error) {
	var models model.Models

	for m, err := range c.Client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing models: %w", err)
		}

		name := strings.TrimPrefix(m.Name, "models/")

		mdl := model.Model{
			Name:           name,
			ParameterCount: model.ParseParameterName(name),
			ContextLength:  model.ContextLength(m.InputTokenLimit),
		}

		enrichFromAPI(&mdl, m.Thinking, m.SupportedActions)

		models = append(models, mdl)
	}

	return models, nil
}
