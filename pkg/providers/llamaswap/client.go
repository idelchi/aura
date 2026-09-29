// Package llamaswap adds llama-swap routing and model control to llama.cpp inference.
package llamaswap

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/llamacpp"
)

// Client shares llama.cpp inference, using llama-swap's per-model native routes.
type Client struct{ *llamacpp.Client }

// New creates a client for a llama-swap server backed by llama.cpp workers.
func New(serverURL, token string, timeout time.Duration) *Client {
	return &Client{Client: llamacpp.New(serverURL, token, timeout,
		llamacpp.WithModelPath(func(name, endpoint string) string {
			return "/upstream/" + url.PathEscape(name) + endpoint
		}),
	)}
}

// LoadModel starts the worker by requesting its properties through the upstream proxy.
func (c *Client) LoadModel(ctx context.Context, name string) error {
	_, err := c.Show(ctx, name)
	return err
}

// UnloadModel stops one worker through llama-swap's control API.
func (c *Client) UnloadModel(ctx context.Context, name string) error {
	endpoint, err := c.WithEndpoint("/api/models/unload/" + url.PathEscape(name))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providers.ClassifyHTTPError(resp.StatusCode, "llamaswap", resp.Status, 0)
	}
	return nil
}
