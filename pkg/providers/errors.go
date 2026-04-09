package providers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrContextExhausted indicates the conversation exceeded the model's context window.
// Providers wrap their provider-specific context-length errors with this sentinel
// so the assistant loop can detect and trigger emergency compaction.
var ErrContextExhausted = errors.New("context length exceeded")

// Provider error sentinels — returned by handleError() in each provider
// so the assistant loop can show actionable user-facing messages.
var (
	ErrRateLimit        = errors.New("rate limited")
	ErrAuth             = errors.New("authentication failed")
	ErrServerError      = errors.New("server error")
	ErrNetwork          = errors.New("network error")
	ErrCreditExhausted  = errors.New("credits exhausted")
	ErrModelUnavailable = errors.New("model unavailable")
	ErrContentFilter    = errors.New("content filtered")

	// ErrEstimateNotSupported is returned when a provider does not support native token estimation.
	ErrEstimateNotSupported = errors.New("provider does not support native token estimation")
)

// RateLimitError carries retry metadata alongside the ErrRateLimit sentinel.
// RetryAfter is parsed from the Retry-After header where available (Anthropic, OpenAI).
// Providers without header access (Google, Ollama, OpenRouter) get DefaultRetryAfter.
type RateLimitError struct {
	RetryAfter time.Duration
	Err        error
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited (retry after %s): %v", e.RetryAfter, e.Err)
	}

	return fmt.Sprintf("rate limited: %v", e.Err)
}

func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimit }
func (e *RateLimitError) Unwrap() error        { return e.Err }

// IsNetworkError reports whether err is a network-level failure (connection refused,
// DNS resolution, timeout) as opposed to an API-level error from the provider SDK.
// Network errors bypass SDK error types, so this must be checked BEFORE SDK assertions.
func IsNetworkError(err error) bool {
	var (
		netErr *net.OpError
		urlErr *url.Error
		dnsErr *net.DNSError
	)

	return errors.As(err, &netErr) ||
		errors.As(err, &urlErr) ||
		errors.As(err, &dnsErr) ||
		errors.Is(err, context.DeadlineExceeded)
}

// WrapNetworkError wraps a network error with the ErrNetwork sentinel.
func WrapNetworkError(err error) error {
	return fmt.Errorf("%w: %w", ErrNetwork, err)
}

// ParseRetryAfter extracts a Retry-After duration from an HTTP response.
// Returns zero if the response is nil or the header is absent/unparseable.
func ParseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}

	val := resp.Header.Get("Retry-After")
	if val == "" {
		return 0
	}

	// Try as seconds first (most common for API rate limits).
	if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}

	// Try HTTP-date format.
	if t, err := http.ParseTime(val); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}

	return 0
}

// DefaultRetryAfter is applied when a provider returns 429 but doesn't expose
// the Retry-After header (Google, OpenRouter, Ollama).
const DefaultRetryAfter = 5 * time.Second

// ClassifyHTTPError maps an HTTP status code to a provider-agnostic sentinel.
// Providers call this AFTER their message-based pre-checks (context exhaustion,
// content filter, credit/quota patterns) which vary per SDK.
// When retryAfter is zero and status is 429, DefaultRetryAfter is applied.
func ClassifyHTTPError(statusCode int, providerName, msg string, retryAfter time.Duration) error {
	switch {
	case statusCode == 429:
		ra := retryAfter
		if ra == 0 {
			ra = DefaultRetryAfter
		}

		return &RateLimitError{RetryAfter: ra, Err: fmt.Errorf("%s: %s", providerName, msg)}
	case statusCode == 401, statusCode == 403:
		return fmt.Errorf("%w: %s", ErrAuth, msg)
	case statusCode == 404:
		return fmt.Errorf("%w: %s", ErrModelUnavailable, msg)
	case statusCode >= 500:
		return fmt.Errorf("%w: %s", ErrServerError, msg)
	}

	return fmt.Errorf("%s: %s", providerName, msg)
}
