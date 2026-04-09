package codex

import (
	"context"
	"net/http"
	"sync"
	"time"

	openaiSDK "github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	openaiProvider "github.com/idelchi/aura/pkg/providers/openai"

	fantasyopenai "charm.land/fantasy/providers/openai"
)

const defaultBaseURL = "https://chatgpt.com/backend-api/codex"

// Client wraps the OpenAI-compatible client with Codex-specific authentication and endpoints.
// Uses a private field (not embedding) to prevent OpenAI's Embed/Transcribe/Synthesize
// methods from being promoted — Codex does not support those capabilities.
type Client struct {
	inner *openaiProvider.Client

	baseURL      string
	mu           sync.Mutex
	cachedModels model.Models
}

// New creates a Codex client using the given URL (optional), refresh token, optional auth file path for token rotation,
// and timeout.
// If url is empty, defaults to the ChatGPT backend API.
func New(url, token, authFilePath string, timeout time.Duration) *Client {
	if url == "" {
		url = defaultBaseURL
	}

	ct := newCodexTransport(token, authFilePath, timeout)
	httpClient := &http.Client{Transport: ct}

	sdkClient := openaiSDK.NewClient(
		openaiOption.WithHTTPClient(httpClient),
		openaiOption.WithBaseURL(url),
	)

	// Fantasy provider for Chat (streaming via Responses API).
	fp, err := fantasyopenai.New(
		fantasyopenai.WithHTTPClient(httpClient),
		fantasyopenai.WithBaseURL(url),
		fantasyopenai.WithUseResponsesAPI(),
	)
	if err != nil {
		debug.Log("[codex] fantasy provider init: %v", err)
	}

	debug.Log("[codex] initialized (url=%s)", url)

	return &Client{
		inner: &openaiProvider.Client{
			Client:     sdkClient,
			HTTPClient: httpClient,
			Fantasy:    fp,
		},
		baseURL: url,
	}
}

// Estimate delegates to the inner OpenAI client's token estimation.
func (c *Client) Estimate(ctx context.Context, req request.Request, text string) (int, error) {
	return c.inner.Estimate(ctx, req, text)
}
