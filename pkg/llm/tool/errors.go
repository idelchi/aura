package tool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

// FileError produces a clean, single-line error for LLM consumption.
// It unwraps through godyl's wrapping layers to the root OS error,
// classifies it, and returns "op path: reason" without stuttering.
//
// Uses %v (not %w) because tool Execute() errors are terminal text
// for the LLM — they are never inspected with errors.Is() by Go callers.
func FileError(op, path string, err error) error {
	inner := innerError(err)

	switch {
	case os.IsNotExist(inner):
		return fmt.Errorf("%s %s: no such file or directory", op, path)
	case os.IsPermission(inner):
		return fmt.Errorf("%s %s: permission denied", op, path)
	case errors.Is(inner, syscall.EISDIR):
		return fmt.Errorf("%s %s: is a directory, not a file", op, path)
	case errors.Is(inner, syscall.ENOSPC):
		return fmt.Errorf("%s %s: disk full", op, path)
	case errors.Is(inner, syscall.EROFS):
		return fmt.Errorf("%s %s: read-only filesystem", op, path)
	default:
		return fmt.Errorf("%s %s: %w", op, path, inner)
	}
}

// NetworkError produces a clean, single-line error for LLM consumption
// from network/HTTP client errors. It classifies timeouts, DNS failures,
// and connection errors into actionable messages.
func NetworkError(host string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("request to %s timed out", host)
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Errorf("cannot reach %s: host not found", host)
	}

	return fmt.Errorf("cannot reach %s: %w", host, innerError(err))
}

// HTTPError produces a clean, single-line error for LLM consumption
// from non-200 HTTP status codes.
func HTTPError(host string, statusCode int) error {
	switch {
	case statusCode == 404:
		return fmt.Errorf("page not found: %s (HTTP 404)", host)
	case statusCode == 403:
		return fmt.Errorf("access denied: %s (HTTP 403)", host)
	case statusCode == 429:
		return fmt.Errorf("rate limited by %s (HTTP 429) — wait and retry", host)
	case statusCode >= 500:
		return fmt.Errorf("server error at %s (HTTP %d)", host, statusCode)
	default:
		return fmt.Errorf("HTTP %d from %s", statusCode, host)
	}
}

// innerError unwraps through godyl's fmt.Errorf wrapping layers
// to reach the root cause. If the chain contains an *os.PathError,
// it returns PathError.Err (the syscall error) to strip the redundant
// path from the message.
func innerError(err error) error {
	for {
		var pe *os.PathError
		if errors.As(err, &pe) {
			return pe.Err
		}

		u := errors.Unwrap(err)
		if u == nil {
			return err
		}

		err = u
	}
}
