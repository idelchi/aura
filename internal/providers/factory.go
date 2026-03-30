// Package providers bridges internal config with the public provider implementations.
package providers

import (
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/auth"
	providers "github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/anthropic"
	"github.com/idelchi/aura/pkg/providers/codex"
	"github.com/idelchi/aura/pkg/providers/copilot"
	"github.com/idelchi/aura/pkg/providers/google"
	"github.com/idelchi/aura/pkg/providers/llamacpp"
	"github.com/idelchi/aura/pkg/providers/ollama"
	"github.com/idelchi/aura/pkg/providers/openai"
	"github.com/idelchi/aura/pkg/providers/openrouter"
)

// Provider is an alias for the public Provider interface.
type Provider = providers.Provider

// ModelLoader is an alias for the public ModelLoader interface.
type ModelLoader = providers.ModelLoader

// Opt-in capability interfaces — aliases for the public types.
type (
	Embedder    = providers.Embedder
	Reranker    = providers.Reranker
	Transcriber = providers.Transcriber
	Synthesizer = providers.Synthesizer
)

// As unwraps provider wrapper layers (RetryProvider, etc.) to find the first
// provider satisfying interface T. Re-exported from pkg/providers.
func As[T any](p Provider) (T, bool) {
	return providers.As[T](p)
}

// New creates a provider instance by name with the given URL and token.
func New(provider config.Provider) (Provider, error) {
	timeout, err := time.ParseDuration(provider.Timeout)
	if err != nil {
		return nil, fmt.Errorf("parsing timeout %q: %w", provider.Timeout, err)
	}

	debug.Log("[provider] creating %s (url=%s timeout=%s)", provider.Type, provider.URL, timeout)

	var p Provider

	switch provider.Type {
	case "openrouter":
		p = openrouter.New(provider.URL, provider.Token, timeout)
	case "ollama":
		var keepAlive time.Duration

		if provider.KeepAlive != "" {
			d, err := time.ParseDuration(provider.KeepAlive)
			if err != nil {
				return nil, fmt.Errorf("parsing keep_alive %q: %w", provider.KeepAlive, err)
			}

			keepAlive = d
		}

		p, err = ollama.New(provider.URL, provider.Token, keepAlive, timeout)
		if err != nil {
			return nil, err
		}
	case "llamacpp":
		p = llamacpp.New(provider.URL, provider.Token, timeout)
	case "openai":
		p = openai.New(provider.URL, provider.Token, timeout)
	case "anthropic":
		p = anthropic.New(provider.URL, provider.Token, timeout)
	case "google":
		p, err = google.New(provider.URL, provider.Token, timeout)
		if err != nil {
			return nil, err
		}
	case "copilot":
		token := provider.Token
		if token == "" {
			t, _, err := auth.Load("copilot", provider.AuthDirs...)
			if err != nil {
				return nil, errors.New(
					"provider copilot requires authentication — run 'aura login copilot' or set AURA_PROVIDERS_COPILOT_TOKEN",
				)
			}

			token = t
		}

		p = copilot.New(token, timeout)
	case "codex":
		token := provider.Token
		authFile := ""

		if token == "" {
			t, path, err := auth.Load("codex", provider.AuthDirs...)
			if err != nil {
				return nil, errors.New(
					"provider codex requires authentication — run 'aura login codex' or set AURA_PROVIDERS_CODEX_TOKEN",
				)
			}

			token = t
			authFile = path
		}

		p = codex.New(provider.URL, token, authFile, timeout)
	default:
		return nil, fmt.Errorf("unknown provider: %q", provider.Type)
	}

	// Fantasy-based providers have built-in retry. Only wrap native providers.
	nativeProvider := provider.Type == "ollama" || provider.Type == "llamacpp"

	if provider.Retry.MaxAttempts > 0 && nativeProvider {
		p, err = wrapRetry(p, provider.Retry)
		if err != nil {
			return nil, fmt.Errorf("configuring retry: %w", err)
		}
	}

	return p, nil
}

func wrapRetry(p Provider, cfg config.Retry) (Provider, error) {
	if cfg.BaseDelay == "" {
		cfg.BaseDelay = "1s"
	}

	if cfg.MaxDelay == "" {
		cfg.MaxDelay = "30s"
	}

	baseDelay, err := time.ParseDuration(cfg.BaseDelay)
	if err != nil {
		return nil, fmt.Errorf("parsing base_delay %q: %w", cfg.BaseDelay, err)
	}

	maxDelay, err := time.ParseDuration(cfg.MaxDelay)
	if err != nil {
		return nil, fmt.Errorf("parsing max_delay %q: %w", cfg.MaxDelay, err)
	}

	bo := backoff.NewExponentialBackOff()

	bo.InitialInterval = baseDelay
	bo.MaxInterval = maxDelay
	bo.MaxElapsedTime = 0 // controlled by MaxAttempts, not elapsed time

	return &providers.RetryProvider{
		Provider: p,
		Bo:       backoff.WithMaxRetries(bo, uint64(cfg.MaxAttempts)),
	}, nil
}
