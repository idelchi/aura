package ollama

import (
	"context"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/pkg/llm/embedding"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

// Embed generates embeddings for the given input texts.
func (c *Client) Embed(ctx context.Context, req embedding.Request) (embedding.Response, usage.Usage, error) {
	truncate := true

	apiReq := &api.EmbedRequest{
		Model:    req.Model,
		Input:    req.Input,
		Truncate: &truncate,
	}

	if c.keepAlive > 0 {
		apiReq.KeepAlive = &api.Duration{Duration: c.keepAlive}
	}

	resp, err := c.Client.Embed(ctx, apiReq)
	if err != nil {
		return embedding.Response{}, usage.Usage{}, handleError(err)
	}

	embeddings := make([][]float64, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		embeddings[i] = providers.Float32sToFloat64s(emb)
	}

	return embedding.Response{
			Model:      resp.Model,
			Embeddings: embeddings,
		}, usage.Usage{
			Input: resp.PromptEvalCount,
		}, nil
}
