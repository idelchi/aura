package providers

import (
	"context"
	"errors"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// RetryProvider wraps a Provider with exponential-backoff retry logic for Chat().
// All other Provider methods pass through to the embedded provider unchanged.
//
// Retry is stream-safe: once the stream callback has been invoked (partial content
// emitted to the terminal), the error is marked permanent and no retry is attempted.
//
// Fields are exported because the factory creating RetryProvider lives in
// internal/providers/ (different package).
type RetryProvider struct {
	Provider // embedded for pass-through of Models, Model, Estimate, etc.

	Bo backoff.BackOff
}

// Chat calls the underlying provider's Chat with retry logic for transient errors.
func (r *RetryProvider) Chat(
	ctx context.Context,
	req request.Request,
	fn stream.Func,
) (message.Message, usage.Usage, error) {
	var (
		msg message.Message
		u   usage.Usage
	)

	called := false

	op := func() error {
		var err error

		// Only wrap when fn is non-nil — subagents pass stream.Func(nil).
		// Wrapping nil would panic when the inner provider calls the callback.
		var wrappedFn stream.Func

		if fn != nil {
			wrappedFn = func(thinking, content string, done bool) error {
				called = true

				return fn(thinking, content, done)
			}
		}

		msg, u, err = r.Provider.Chat(ctx, req, wrappedFn)
		if err == nil {
			return nil
		}

		// Once streaming started, can't retry — partial output already emitted to terminal.
		if called {
			return backoff.Permanent(err)
		}

		if !isRetryable(err) {
			return backoff.Permanent(err)
		}

		// Honor server-supplied Retry-After as minimum delay.
		// Total wait = RetryAfter + backoff delay (conservative — more polite to the API).
		var rle *RateLimitError
		if errors.As(err, &rle) && rle.RetryAfter > 0 {
			time.Sleep(rle.RetryAfter)
		}

		return err
	}

	notify := func(err error, d time.Duration) {
		debug.Log("[retry] retrying in %s: %v", d, err)
	}

	err := backoff.RetryNotify(op, backoff.WithContext(r.Bo, ctx), notify)

	return msg, u, err
}

// Unwrap returns the inner provider, enabling providers.As[T]() to find
// opt-in interfaces through the retry wrapper.
func (r *RetryProvider) Unwrap() Provider { return r.Provider }

// isRetryable reports whether the error is a transient failure worth retrying.
func isRetryable(err error) bool {
	return errors.Is(err, ErrRateLimit) ||
		errors.Is(err, ErrServerError) ||
		errors.Is(err, ErrNetwork)
}
