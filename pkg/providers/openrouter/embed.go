package openrouter

import (
	"context"

	"github.com/revrost/go-openrouter"

	"github.com/idelchi/aura/pkg/llm/embedding"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers/adapter"
)

// Embed generates embeddings for the given input texts.
func (c *Client) Embed(ctx context.Context, req embedding.Request) (embedding.Response, usage.Usage, error) {
	apiReq := openrouter.EmbeddingsRequest{
		Model: req.Model,
		Input: req.Input,
	}

	resp, err := c.CreateEmbeddings(ctx, apiReq)
	if err != nil {
		return embedding.Response{}, usage.Usage{}, adapter.MapError(err)
	}

	embeddings := make([][]float64, len(resp.Data))
	for i, data := range resp.Data {
		embeddings[i] = data.Embedding.Vector
	}

	var u usage.Usage

	if resp.Usage != nil {
		u.Input = resp.Usage.PromptTokens
	}

	return embedding.Response{
		Model:      resp.Model,
		Embeddings: embeddings,
	}, u, nil
}
