package openai

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/catalog"
	"github.com/idelchi/aura/pkg/providers/registry"
)

// List fetches the list of available models using the OpenAI SDK.
func (c *Client) List(ctx context.Context) (model.Models, error) {
	page, err := c.Client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}

	var models model.Models

	for _, m := range page.Data {
		mdl, err := catalog.Decode([]byte(m.RawJSON()))
		if err != nil {
			return nil, fmt.Errorf("decoding model %q metadata: %w", m.ID, err)
		}

		registry.Enrich("openai", &mdl)

		models = append(models, mdl)
	}

	return models, nil
}
