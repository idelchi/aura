package providers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

// failing always fails a chat with the given error, counting attempts.
type failing struct {
	err      error
	attempts int
}

func (f *failing) Chat(context.Context, request.Request, stream.Func) (message.Message, usage.Usage, error) {
	f.attempts++

	return message.Message{}, usage.Usage{}, f.err
}

func (f *failing) Models(context.Context) (model.Models, error)                   { return nil, nil }
func (f *failing) Model(context.Context, string) (model.Model, error)             { return model.Model{}, nil }
func (f *failing) Estimate(context.Context, request.Request, string) (int, error) { return 0, nil }

func TestRetryWaitHonoursCancellation(t *testing.T) {
	limited := providers.ClassifyHTTPError(429, "llamaswap", "slow down", time.Minute)
	inner := &failing{err: limited}
	retry := &providers.RetryProvider{Provider: inner, Bo: backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Millisecond), 3)}

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	_, _, err := retry.Chat(ctx, request.Request{}, nil)

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancellation waited out Retry-After: %v", elapsed)
	}

	if !errors.Is(err, context.Canceled) || !errors.Is(err, providers.ErrRateLimit) || inner.attempts != 1 {
		t.Fatalf("err = %v after %d attempts; want the cancellation and its cause, one attempt", err, inner.attempts)
	}
}

func TestRetryRetriesTypedServerErrors(t *testing.T) {
	stopped := providers.ClassifyHTTPError(503, "llamaswap", "the engine is not running", 0)
	inner := &failing{err: stopped}
	retry := &providers.RetryProvider{Provider: inner, Bo: backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Millisecond), 2)}

	_, _, err := retry.Chat(t.Context(), request.Request{}, nil)
	if !errors.Is(err, providers.ErrServerError) || inner.attempts != 3 {
		t.Fatalf("err = %v after %d attempts, want 3 attempts of a server error", err, inner.attempts)
	}
}
