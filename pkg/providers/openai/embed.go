package openai

import (
	"context"

	"github.com/openai/openai-go/v3"

	"github.com/idelchi/aura/pkg/llm/embedding"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers/adapter"
)

// Embed generates embeddings for the given input texts.
func (c *Client) Embed(ctx context.Context, req embedding.Request) (embedding.Response, usage.Usage, error) {
	params := openai.EmbeddingNewParams{
		Model: req.Model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: req.Input,
		},
	}

	resp, err := c.Embeddings.New(ctx, params)
	if err != nil {
		return embedding.Response{}, usage.Usage{}, adapter.MapError(err)
	}

	embeddings := make([][]float64, len(resp.Data))
	for i, data := range resp.Data {
		embeddings[i] = data.Embedding
	}

	return embedding.Response{
			Model:      resp.Model,
			Embeddings: embeddings,
		}, usage.Usage{
			Input: int(resp.Usage.PromptTokens),
		}, nil
}
