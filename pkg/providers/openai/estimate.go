package openai

import (
	"context"
	"errors"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/adapter"
)

// Estimate tokenizes content using OpenAI's InputTokens.Count API.
// Passes the raw text string as input and returns the exact token count
// from the model's tokenizer. Falls back to local estimation on any error.
func (c *Client) Estimate(ctx context.Context, req request.Request, content string) (int, error) {
	resp, err := c.Responses.InputTokens.Count(ctx, responses.InputTokenCountParams{
		Model: param.NewOpt(req.Model.Name),
		Input: responses.InputTokenCountParamsInputUnion{
			OfString: param.NewOpt(content),
		},
	})
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusNotFound, http.StatusNotImplemented, http.StatusMethodNotAllowed:
				return 0, providers.ErrEstimateNotSupported
			default:
				return 0, providers.ClassifyHTTPError(apiErr.StatusCode, "openai", apiErr.Message, 0)
			}
		}
		return 0, adapter.MapError(err)
	}

	return int(resp.InputTokens), nil
}
