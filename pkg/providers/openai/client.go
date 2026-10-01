package openai

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers/transport"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

// Client wraps the OpenAI SDK client with Fantasy for chat.
type Client struct {
	openai.Client

	HTTPClient *http.Client

	Fantasy        fantasy.Provider
	fantasyOptions []fantasyopenai.Option
}

const defaultBaseURL = "https://api.openai.com/v1"

// New creates an OpenAI-compatible client for the given base URL, token, and response timeout.
// If baseURL is empty, defaults to the OpenAI API.
func New(baseURL, token string, timeout time.Duration, headers map[string]string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	opts := []transport.Option{transport.WithTimeout(timeout)}

	if token != "" {
		opts = append(opts, transport.WithToken(token))
	}

	tr := transport.New(opts...)

	httpClient := &http.Client{Transport: tr}

	sdkOpts := []option.RequestOption{
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(httpClient),
	}

	if token != "" {
		sdkOpts = append(sdkOpts, option.WithAPIKey(token))
	}
	for name, value := range headers {
		sdkOpts = append(sdkOpts, option.WithHeader(name, value))
	}

	// Fantasy provider for Chat (streaming via Responses API).
	fpOpts := []fantasyopenai.Option{
		fantasyopenai.WithHTTPClient(httpClient),
		fantasyopenai.WithBaseURL(baseURL),
		fantasyopenai.WithUseResponsesAPI(),
		fantasyopenai.WithHeaders(headers),
		fantasyopenai.WithLanguageModelOptions(fantasyopenai.WithLanguageModelStreamExtraFunc(streamReasoning)),
	}

	if token != "" {
		fpOpts = append(fpOpts, fantasyopenai.WithAPIKey(token))
	}

	fp, err := fantasyopenai.New(fpOpts...)
	if err != nil {
		debug.Log("[openai] fantasy provider init: %v", err)
	}

	debug.Log("[openai] initialized (url=%s)", baseURL)

	return &Client{
		Client:         openai.NewClient(sdkOpts...),
		HTTPClient:     httpClient,
		Fantasy:        fp,
		fantasyOptions: fpOpts,
	}
}

// LanguageModel applies request-local extensions without changing the shared client.
// Numeric thinking budgets are an extension supported by compatible servers, not the OpenAI API.
func (c *Client) LanguageModel(ctx context.Context, req request.Request) (fantasy.LanguageModel, error) {
	provider := c.Fantasy
	if g := req.Generation; g != nil && g.ThinkBudget != nil {
		opts := append(slices.Clone(c.fantasyOptions), fantasyopenai.WithSDKOptions(
			option.WithJSONSet("thinking_budget_tokens", *g.ThinkBudget),
		))
		var err error
		provider, err = fantasyopenai.New(opts...)
		if err != nil {
			return nil, err
		}
	}
	return provider.LanguageModel(ctx, req.Model.Name)
}
