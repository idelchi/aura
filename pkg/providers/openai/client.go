package openai

import (
	"net/http"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/providers/transport"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

// Client wraps the OpenAI SDK client with Fantasy for chat.
type Client struct {
	openai.Client

	HTTPClient *http.Client

	Fantasy fantasy.Provider
}

const defaultBaseURL = "https://api.openai.com/v1"

// New creates an OpenAI-compatible client for the given base URL, token, and response timeout.
// If baseURL is empty, defaults to the OpenAI API.
func New(baseURL, token string, timeout time.Duration) *Client {
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

	// Fantasy provider for Chat (streaming via Responses API).
	fpOpts := []fantasyopenai.Option{
		fantasyopenai.WithHTTPClient(httpClient),
		fantasyopenai.WithBaseURL(baseURL),
		fantasyopenai.WithUseResponsesAPI(),
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
		Client:     openai.NewClient(sdkOpts...),
		HTTPClient: httpClient,
		Fantasy:    fp,
	}
}
