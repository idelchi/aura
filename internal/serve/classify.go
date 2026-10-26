package serve

import (
	"context"
	"errors"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/pkg/automation"
	"github.com/idelchi/aura/pkg/providers"
)

// Classify maps an engine error to its automation error. Nil maps to nil.
func Classify(err error) *automation.Error {
	if err == nil {
		return nil
	}

	if e, ok := errors.AsType[*automation.Error](err); ok {
		return e
	}

	code := classifyCode(err)

	out := &automation.Error{Code: code, Message: err.Error(), Retryable: code.Retryable()}

	if status, ok := errors.AsType[*providers.StatusError](err); ok {
		out.Details.Status = status.Status
		out.Details.Provider = status.Provider
	}

	if limited, ok := errors.AsType[*providers.RateLimitError](err); ok {
		out.Details.RetryAfterMS = limited.RetryAfter.Milliseconds()
	}

	return out
}

// classifyCode selects the most specific code. Engine-level causes win over the
// provider class they wrap (a persistence failure is not a provider failure).
func classifyCode(err error) automation.Code {
	switch {
	case errors.Is(err, assistant.ErrPersist):
		return automation.CodePersistFailed
	case errors.Is(err, context.Canceled):
		return automation.CodeCancelled
	case errors.Is(err, session.ErrNotFound):
		return automation.CodeNotFound
	case errors.Is(err, session.ErrExists):
		return automation.CodeExists
	case errors.Is(err, session.ErrFrozen):
		return automation.CodeSessionFrozen
	case errors.Is(err, session.ErrLocked):
		return automation.CodeSessionLocked
	case errors.Is(err, session.ErrRevisionMismatch):
		return automation.CodeRevisionMismatch
	case errors.Is(err, session.ErrInvalidID):
		return automation.CodeInvalidID
	case errors.Is(err, session.ErrInvalidSession):
		return automation.CodeInvalidSession
	case errors.Is(err, assistant.ErrInputRejected):
		return automation.CodeInputRejected
	case errors.Is(err, assistant.ErrGuardrailBlocked):
		return automation.CodeGuardrailBlocked
	case errors.Is(err, assistant.ErrPolicyStopped):
		return automation.CodePolicyStop
	case errors.Is(err, assistant.ErrNotCommand):
		return automation.CodeCommandFailed
	case errors.Is(err, assistant.ErrMaxSteps):
		return automation.CodeMaxSteps
	case errors.Is(err, assistant.ErrTokenBudget):
		return automation.CodeTokenBudget
	case errors.Is(err, assistant.ErrCompactionExhausted), errors.Is(err, assistant.ErrCompactionConfig):
		return automation.CodeCompactionFailed
	case errors.Is(err, providers.ErrContextExhausted):
		return automation.CodeContextExhausted
	case errors.Is(err, providers.ErrRateLimit):
		return automation.CodeProviderRateLimit
	case errors.Is(err, providers.ErrAuth):
		return automation.CodeProviderAuth
	case errors.Is(err, providers.ErrCreditExhausted):
		return automation.CodeProviderCredit
	case errors.Is(err, providers.ErrModelUnavailable):
		return automation.CodeProviderModelUnavailable
	case errors.Is(err, providers.ErrContentFilter):
		return automation.CodeContentFilter
	case errors.Is(err, providers.ErrNetwork):
		return automation.CodeProviderNetwork
	case errors.Is(err, providers.ErrServerError):
		return automation.CodeProviderServer
	default:
		return automation.CodeInternal
	}
}
