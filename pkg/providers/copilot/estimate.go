package copilot

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers"
)

// Estimate is not supported by the Copilot provider.
func (c *Client) Estimate(_ context.Context, _ request.Request, _ string) (int, error) {
	return 0, providers.ErrEstimateNotSupported
}
