package noop

import (
	"context"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers"
)

// Estimate is not supported by the noop provider.
func (p *Provider) Estimate(_ context.Context, _ request.Request, _ string) (int, error) {
	return 0, providers.ErrEstimateNotSupported
}
