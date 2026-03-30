package copilot

import (
	"net/http"
	"sync"
	"time"

	anthropicSDK "github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	openaiSDK "github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"

	"github.com/idelchi/aura/internal/debug"
	anthropicProvider "github.com/idelchi/aura/pkg/providers/anthropic"
	openaiProvider "github.com/idelchi/aura/pkg/providers/openai"

	"charm.land/fantasy"
	fanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

// Client is the GitHub Copilot provider. It routes requests to either an embedded
// OpenAI or Anthropic client based on each model's supported endpoints.
type Client struct {
	openai     *openaiProvider.Client
	anthropic  *anthropicProvider.Client
	httpClient *http.Client
	transport  *copilotTransport
	mu         sync.Mutex
	models     map[string]modelInfo
}

// New creates a Copilot client authenticating with the given GitHub OAuth token.
func New(token string, timeout time.Duration) *Client {
	tr := newTransport(token, timeout)
	httpClient := &http.Client{Transport: tr}

	const placeholder = "https://copilot.placeholder"

	// Legacy SDK clients (for non-chat methods: models, estimate)
	openaiClient := openaiSDK.NewClient(
		openaiOption.WithHTTPClient(httpClient),
		openaiOption.WithBaseURL(placeholder),
	)

	anthropicClient := anthropicSDK.NewClient(
		anthropicOption.WithHTTPClient(httpClient),
		anthropicOption.WithBaseURL(placeholder),
		anthropicOption.WithAPIKey("placeholder"),
	)

	// Fantasy providers for Chat (streaming)
	fantasyOpenAI := initFantasyProvider(fantasyopenai.New(
		fantasyopenai.WithHTTPClient(httpClient),
		fantasyopenai.WithBaseURL(placeholder),
		fantasyopenai.WithUseResponsesAPI(),
	))

	fantasyAnthropic := initFantasyProvider(fanthropic.New(
		fanthropic.WithHTTPClient(httpClient),
		fanthropic.WithBaseURL(placeholder),
		fanthropic.WithSkipAuth(true),
	))

	debug.Log("[copilot] initialized")

	return &Client{
		openai: &openaiProvider.Client{
			Client:     openaiClient,
			HTTPClient: httpClient,
			Fantasy:    fantasyOpenAI,
		},
		anthropic: &anthropicProvider.Client{
			Client:  anthropicClient,
			Fantasy: fantasyAnthropic,
		},
		httpClient: httpClient,
		transport:  tr,
	}
}

// initFantasyProvider logs and returns the provider, handling init errors.
func initFantasyProvider(fp fantasy.Provider, err error) fantasy.Provider {
	if err != nil {
		debug.Log("[copilot] fantasy provider init: %v", err)
	}

	return fp
}
