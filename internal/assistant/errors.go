package assistant

import (
	"errors"

	"github.com/idelchi/aura/pkg/providers"
)

// classifyError maps a provider error to a classification string for plugin enrichment.
// Note: ErrContextExhausted is included for completeness but is unreachable from OnError —
// it is caught and handled by compaction recovery before INJECTION POINT 4.
func classifyError(err error) string {
	switch {
	case errors.Is(err, providers.ErrRateLimit):
		return "rate_limit"
	case errors.Is(err, providers.ErrAuth):
		return "auth"
	case errors.Is(err, providers.ErrContextExhausted):
		return "context_exhausted"
	case errors.Is(err, providers.ErrNetwork):
		return "network"
	case errors.Is(err, providers.ErrServerError):
		return "server"
	case errors.Is(err, providers.ErrContentFilter):
		return "content_filter"
	case errors.Is(err, providers.ErrCreditExhausted):
		return "credit_exhausted"
	case errors.Is(err, providers.ErrModelUnavailable):
		return "model_unavailable"
	default:
		return ""
	}
}

// isRetryable reports whether an error is transient and worth retrying.
// Mirrors pkg/providers/retry.go isRetryable() — intentional duplication
// at the assistant layer for plugin enrichment (different purpose than provider retry).
func isRetryable(err error) bool {
	return errors.Is(err, providers.ErrRateLimit) ||
		errors.Is(err, providers.ErrServerError) ||
		errors.Is(err, providers.ErrNetwork)
}
