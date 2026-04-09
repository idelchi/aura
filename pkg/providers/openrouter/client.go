package openrouter

import (
	"net/http"
	"sync"
	"time"

	"github.com/revrost/go-openrouter"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/transport"

	"charm.land/fantasy"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
)

// Client wraps the OpenRouter API client with Fantasy for chat.
type Client struct {
	*openrouter.Client

	fantasy fantasy.Provider

	mu           sync.Mutex
	cachedModels model.Models
}

// New creates an OpenRouter client with the given URL, token, and response timeout.
func New(url, token string, timeout time.Duration) *Client {
	cfg := openrouter.DefaultConfig(token)

	if url != "" {
		cfg.BaseURL = url
	}

	opts := []transport.Option{transport.WithTimeout(timeout)}

	if token != "" {
		opts = append(opts, transport.WithToken(token))
	}

	httpClient := &http.Client{Transport: transport.New(opts...)}

	cfg.HTTPClient = httpClient

	// Fantasy provider for Chat (streaming)
	fpOpts := []fantasyopenrouter.Option{
		fantasyopenrouter.WithHTTPClient(httpClient),
	}

	if token != "" {
		fpOpts = append(fpOpts, fantasyopenrouter.WithAPIKey(token))
	}

	fp, err := fantasyopenrouter.New(fpOpts...)
	if err != nil {
		debug.Log("[openrouter] fantasy provider init: %v", err)
	}

	debug.Log("[openrouter] initialized (url=%s)", url)

	return &Client{
		Client:  openrouter.NewClientWithConfig(*cfg),
		fantasy: fp,
	}
}
