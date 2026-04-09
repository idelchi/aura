package google

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/pkg/llm/embedding"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/adapter"

	"google.golang.org/genai"
)

// Embed generates embeddings for the given input texts.
func (c *Client) Embed(ctx context.Context, req embedding.Request) (embedding.Response, usage.Usage, error) {
	var contents []*genai.Content

	for _, text := range req.Input {
		contents = append(contents, genai.NewContentFromText(text, ""))
	}

	resp, err := c.Client.Models.EmbedContent(ctx, req.Model, contents, nil)
	if err != nil {
		return embedding.Response{}, usage.Usage{}, adapter.MapError(err)
	}

	embeddings := make([][]float64, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		if emb == nil || emb.Values == nil {
			return embedding.Response{}, usage.Usage{}, fmt.Errorf("embedding %d returned nil values", i)
		}

		embeddings[i] = providers.Float32sToFloat64s(emb.Values)
	}

	// Gemini's EmbedContentResponse only provides BillableCharacterCount, not token counts.
	return embedding.Response{
		Model:      req.Model,
		Embeddings: embeddings,
	}, usage.Usage{}, nil
}
