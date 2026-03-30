package ollama

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/model"
)

// Model fetches metadata for the specified model.
func (c *Client) Model(ctx context.Context, name string) (model.Model, error) {
	debug.Log("[ollama] Model() called for %q", name)

	info, err := c.Show(ctx, &api.ShowRequest{Model: name})
	if err != nil {
		return model.Model{}, fmt.Errorf("fetching model info for %q: %w", name, handleError(err))
	}

	ctxLen := contextLength(ModelInfo(info.ModelInfo))

	debug.Log("[ollama] Model() %q: contextLength=%d family=%s", name, int(ctxLen), info.Details.Family)

	params := model.ParseParameterSize(info.Details.ParameterSize)
	if params == 0 {
		params = model.ParseParameterName(name)
	}

	// Size is not set here — the Show() endpoint doesn't return file size;
	// only the List() endpoint includes it.
	m := model.Model{
		Name:           name,
		ParameterCount: params,
		ContextLength:  ctxLen,
		Family:         info.Details.Family,
	}

	return WithCapabilities(m, info), nil
}
