package tokens

import (
	"context"
	"fmt"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

// Estimator provides configurable token estimation with eagerly-initialized tiktoken encoding.
// Thread-safe: tiktoken-go is not safe for concurrent use, so encoding is guarded by a mutex.
//
// Two estimation paths:
//   - Estimate(ctx, text) — uses native provider estimation when available, falls back to local.
//   - EstimateLocal(text) — always uses local methods (rough/tiktoken). Never calls the provider.
type Estimator struct {
	method   string
	divisor  int
	mu       sync.Mutex
	enc      *tiktoken.Tiktoken
	native   func(context.Context, string) (int, error)
	debugLog func(string, ...any)
}

// NewEstimator creates an Estimator with the given method, encoding, and divisor.
// Valid methods: "rough", "tiktoken", "rough+tiktoken", "native".
// The "native" method falls back to "rough+tiktoken" for EstimateLocal calls —
// actual native estimation requires UseNative() to wire the provider.
//
// If the method uses tiktoken (anything except "rough"), the encoding is initialized
// eagerly and an error is returned on failure.
func NewEstimator(method, encoding string, divisor int) (*Estimator, error) {
	e := &Estimator{
		method:  method,
		divisor: divisor,
	}

	if method != "rough" {
		enc, err := tiktoken.GetEncoding(encoding)
		if err != nil {
			return nil, fmt.Errorf("tiktoken encoding %q: %w", encoding, err)
		}

		e.enc = enc
	}

	return e, nil
}

// UseNative registers a function that calls the provider's native estimation endpoint.
// The Estimator falls back to local estimation if the native function returns an error.
func (e *Estimator) UseNative(fn func(context.Context, string) (int, error)) {
	e.mu.Lock()
	e.native = fn
	e.mu.Unlock()
}

// HasNative reports whether a native estimation function has been registered.
func (e *Estimator) HasNative() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.native != nil
}

// SetDebug registers a debug logging function for per-call estimation breakdowns.
func (e *Estimator) SetDebug(fn func(string, ...any)) {
	e.mu.Lock()
	e.debugLog = fn
	e.mu.Unlock()
}

// EstimateBreakdown holds per-method token estimates for debug logging.
type EstimateBreakdown struct {
	Rough         int
	Tiktoken      int
	RoughTiktoken int
}

// EstimateAll returns token estimates for all methods (rough, tiktoken, rough+tiktoken).
// Used for debug logging — the "winner" depends on the configured method.
func (e *Estimator) EstimateAll(text string) EstimateBreakdown {
	if len(text) == 0 {
		return EstimateBreakdown{}
	}

	r := e.Rough(text)
	t := e.Tiktoken(text)

	return EstimateBreakdown{
		Rough:         r,
		Tiktoken:      t,
		RoughTiktoken: max(r, t),
	}
}

// Method returns the configured estimation method name.
func (e *Estimator) Method() string {
	return e.method
}

// Estimate returns a token count using native provider estimation when available,
// falling back to local estimation on error or when native is not configured.
// Use for low-frequency, accuracy-critical call sites (builder, result guard).
func (e *Estimator) Estimate(ctx context.Context, text string) int {
	if len(text) == 0 {
		return 0
	}

	// Snapshot function pointers under lock, then release before any calls.
	e.mu.Lock()
	native := e.native
	debug := e.debugLog
	e.mu.Unlock()

	bd := e.EstimateAll(text)

	if native != nil {
		count, err := native(ctx, text)
		if err == nil {
			if debug != nil {
				debug("[estimate] method=native chars=%d result=%d rough=%d tiktoken=%d rough+tiktoken=%d",
					len(text), count, bd.Rough, bd.Tiktoken, bd.RoughTiktoken)
			}

			return count
		}

		// Native failed — fall through to local.
	}

	result := e.EstimateLocal(text)

	if debug != nil {
		debug("[estimate] method=%s chars=%d result=%d rough=%d tiktoken=%d rough+tiktoken=%d",
			e.method, len(text), result, bd.Rough, bd.Tiktoken, bd.RoughTiktoken)
	}

	return result
}

// EstimateLocal returns a token count for text using the configured local method.
// Never calls the provider — use for high-frequency estimation (chunking, pruning).
func (e *Estimator) EstimateLocal(text string) int {
	if len(text) == 0 {
		return 0
	}

	switch e.method {
	case "rough":
		return e.Rough(text)
	case "tiktoken":
		return e.Tiktoken(text)
	case "rough+tiktoken", "native":
		return max(e.Rough(text), e.Tiktoken(text))
	default:
		return max(e.Rough(text), e.Tiktoken(text))
	}
}

// Rough returns a character-based token estimate using the configured divisor.
func (e *Estimator) Rough(text string) int {
	return max(len(text)/e.divisor, 1)
}

// Tiktoken returns a token count using the configured tiktoken encoding.
// Returns 0 when enc is nil (method "rough" skips tiktoken init).
func (e *Estimator) Tiktoken(text string) int {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enc == nil {
		return 0
	}

	return len(e.enc.Encode(text, nil, nil))
}
