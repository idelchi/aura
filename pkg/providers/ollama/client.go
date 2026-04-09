package ollama

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/internal/debug"
	providers "github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/transport"
)

// handleError unwraps ollama SDK errors and classifies them into provider-agnostic sentinels.
func handleError(err error) error {
	if providers.IsNetworkError(err) {
		return providers.WrapNetworkError(err)
	}

	var ae api.AuthorizationError
	if errors.As(err, &ae) {
		msg := ae.Status
		if msg == "" {
			msg = http.StatusText(ae.StatusCode)
		}

		return fmt.Errorf("%w: ollama: %d %s", providers.ErrAuth, ae.StatusCode, msg)
	}

	var se api.StatusError
	if errors.As(err, &se) {
		if strings.Contains(se.ErrorMessage, "input length exceeds") {
			return fmt.Errorf("%w: ollama: %d %s", providers.ErrContextExhausted, se.StatusCode, se.ErrorMessage)
		}

		msg := se.ErrorMessage
		if msg == "" {
			msg = http.StatusText(se.StatusCode)
		}

		// Ollama may return "not found" without a 404 status code.
		if strings.Contains(strings.ToLower(msg), "not found") {
			return fmt.Errorf("%w: ollama: %d %s", providers.ErrModelUnavailable, se.StatusCode, msg)
		}

		return providers.ClassifyHTTPError(se.StatusCode, "ollama", fmt.Sprintf("%d %s", se.StatusCode, msg), 0)
	}

	return err
}

// Client wraps the Ollama API client.
type Client struct {
	*api.Client

	keepAlive time.Duration
}

// New creates an Ollama client for the given server URL, optional token, keepalive, and response timeout.
func New(server, token string, keepAlive, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	opts := []transport.Option{transport.WithTimeout(timeout)}

	if token != "" {
		opts = append(opts, transport.WithToken(token))
	}

	tr := transport.New(opts...)

	debug.Log("[ollama] initialized (url=%s)", server)

	return &Client{
		Client:    api.NewClient(u, &http.Client{Transport: tr}),
		keepAlive: keepAlive,
	}, nil
}
