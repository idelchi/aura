package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/debug"
	openaiProvider "github.com/idelchi/aura/pkg/providers/openai"
)

// Client wraps the OpenAI-compatible client with llama.cpp-specific extensions.
type Client struct {
	*openaiProvider.Client

	baseURL string
}

// New creates a llama.cpp client for the given server URL, optional token, and response timeout.
func New(serverURL, token string, timeout time.Duration) *Client {
	apiURL := strings.TrimSuffix(serverURL, "/") + "/v1"

	debug.Log("[llamacpp] initialized (url=%s)", serverURL)

	return &Client{
		Client:  openaiProvider.New(apiURL, token, timeout),
		baseURL: serverURL,
	}
}

// WithEndpoint constructs a full URL for the given endpoint path.
func (c Client) WithEndpoint(endpoint string) (*url.URL, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	u.Path = path.Join(u.Path, endpoint)

	return u, nil
}

// modelRequest is the payload for load/unload endpoints.
type modelRequest struct {
	Model string `json:"model"`
}

// modelResponse is the response from load/unload endpoints.
type modelResponse struct {
	Success bool `json:"success"`
}

// LoadModel explicitly loads a model on the llama.cpp server (router mode).
func (c *Client) LoadModel(ctx context.Context, name string) error {
	return c.modelAction(ctx, "load", name)
}

// UnloadModel explicitly unloads a model from the llama.cpp server (router mode).
func (c *Client) UnloadModel(ctx context.Context, name string) error {
	return c.modelAction(ctx, "unload", name)
}

// modelAction performs a load or unload request against the llama.cpp router.
func (c *Client) modelAction(ctx context.Context, action, name string) error {
	u, err := c.WithEndpoint("/models/" + action)
	if err != nil {
		return fmt.Errorf("building URL: %w", err)
	}

	body, err := json.Marshal(modelRequest{Model: name})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%sing model: %w", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr Error
		if err := apiErr.FromResponse(resp); err != nil {
			return fmt.Errorf("%s model failed with status %d", action, resp.StatusCode)
		}

		return &apiErr
	}

	var result modelResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("%s model %q: server returned success=false", action, name)
	}

	return nil
}
