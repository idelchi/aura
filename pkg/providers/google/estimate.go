package google

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers/adapter"

	"google.golang.org/genai"
)

// Estimate tokenizes content using Google's CountTokens API.
// Wraps content as a single user content block and returns the exact token count.
// Must use c.Client.Models (not c.Models) to access the genai service field
// rather than Aura's Models() method. Falls back to local estimation on any error.
func (c *Client) Estimate(ctx context.Context, req request.Request, content string) (int, error) {
	resp, err := c.Client.Models.CountTokens(ctx, req.Model.Name, []*genai.Content{
		genai.NewContentFromText(content, "user"),
	}, nil)
	if err != nil {
		return 0, adapter.MapError(err)
	}

	return int(resp.TotalTokens), nil
}
