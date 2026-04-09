package openrouter

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

// Models fetches all available models including chat and embedding models.
func (c *Client) Models(ctx context.Context) (model.Models, error) {
	llms, err := c.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}

	var models model.Models

	for _, llm := range llms {
		var contextLen int64

		if llm.ContextLength != nil {
			contextLen = *llm.ContextLength
		}

		var family string

		if llm.CanonicalSlug != nil {
			family = strings.Split(*llm.CanonicalSlug, "/")[0]
		}

		m := model.Model{
			Name:           llm.ID,
			ParameterCount: model.ParseParameterName(llm.ID),
			ContextLength:  model.ContextLength(contextLen),
			Family:         family,
		}

		m = WithCapabilities(m, llm.SupportedParameters, llm.Architecture.InputModalities)

		models = append(models, m)
	}

	embeddingModels, err := c.ListEmbeddingsModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing embedding models: %w", err)
	}

	for _, embeddingModel := range embeddingModels {
		var contextLen int64

		if embeddingModel.ContextLength != nil {
			contextLen = *embeddingModel.ContextLength
		}

		var family string

		if embeddingModel.CanonicalSlug != nil {
			family = strings.Split(*embeddingModel.CanonicalSlug, "/")[0]
		}

		m := model.Model{
			Name:           embeddingModel.ID,
			ParameterCount: model.ParseParameterName(embeddingModel.ID),
			ContextLength:  model.ContextLength(contextLen),
			Capabilities:   capabilities.Capabilities{capabilities.Embedding},
			Family:         family,
		}

		models = append(models, m)
	}

	return models, nil
}
