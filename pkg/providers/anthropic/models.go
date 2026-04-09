package anthropic

import (
	"context"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/registry"
)

// Models lists all available models from the Anthropic API.
func (c *Client) Models(ctx context.Context) (model.Models, error) {
	pager := c.Client.Models.ListAutoPaging(ctx, sdk.ModelListParams{})

	var models model.Models

	for pager.Next() {
		info := pager.Current()

		m := model.Model{
			Name:           info.ID,
			ParameterCount: model.ParseParameterName(info.ID),
		}

		registry.Enrich("anthropic", &m)

		models = append(models, m)
	}

	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}

	return models, nil
}
