package ollama

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/pkg/llm/model"
)

// Models fetches all available models with their metadata.
func (c *Client) Models(ctx context.Context) (model.Models, error) {
	allModels, err := c.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching models: %w", err)
	}

	g := errgroup.Group{}
	g.SetLimit(100)

	var result model.Models

	var mu sync.Mutex

	for _, m := range allModels.Models {
		g.Go(func() error {
			info, err := c.Show(ctx, &api.ShowRequest{Model: m.Model})
			if err != nil {
				return fmt.Errorf("getting model info for %q: %w", m.Model, handleError(err))
			}

			mu.Lock()
			defer mu.Unlock()

			params := model.ParseParameterSize(info.Details.ParameterSize)
			if params == 0 {
				params = model.ParseParameterName(m.Model)
			}

			model := model.Model{
				Name:           m.Model,
				ParameterCount: params,
				ContextLength:  contextLength(info.ModelInfo),
				Size:           uint64(m.Size),
				Family:         info.Details.Family,
			}

			model = WithCapabilities(model, info)

			result = append(result, model)

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, handleError(err)
	}

	return result, nil
}
