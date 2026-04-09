// Package devicecode implements the shared device code polling loop for OAuth flows.
package devicecode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrPending indicates the user has not yet authorized.
var ErrPending = errors.New("authorization pending")

// ErrSlowDown indicates the poll interval should be increased.
var ErrSlowDown = errors.New("slow down")

// Config describes a device code polling session.
type Config struct {
	// UserCode is the code the user must enter in the browser.
	UserCode string
	// VerificationURL is the URL the user must visit.
	VerificationURL string
	// Interval is the minimum seconds between poll attempts.
	Interval int
	// ExpiresIn is the total seconds before the device code expires.
	ExpiresIn int
	// Poll is called on each iteration. It must return the result token or a sentinel error.
	// Return ErrPending to continue polling, ErrSlowDown to increase interval.
	Poll func(ctx context.Context) (string, error)
}

// Run executes the device code polling loop, printing instructions to stderr.
func Run(ctx context.Context, cfg Config) (string, error) {
	fmt.Fprintf(os.Stderr, "\nEnter code: %s\n", cfg.UserCode)
	fmt.Fprintf(os.Stderr, "Open:       %s\n\n", cfg.VerificationURL)

	interval := max(cfg.Interval, 5)

	expiresIn := cfg.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 900 // 15 minutes default
	}

	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	fmt.Fprint(os.Stderr, "Waiting for authorization")

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}

		fmt.Fprint(os.Stderr, ".")

		token, err := cfg.Poll(ctx)

		switch {
		case err == nil:
			fmt.Fprintln(os.Stderr, " done!")

			return token, nil
		case errors.Is(err, ErrPending):
			continue
		case errors.Is(err, ErrSlowDown):
			interval += 5

			continue
		default:
			fmt.Fprintln(os.Stderr)

			return "", err
		}
	}

	fmt.Fprintln(os.Stderr)

	return "", fmt.Errorf("device code expired after %ds", expiresIn)
}
