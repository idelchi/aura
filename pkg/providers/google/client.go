package google

import (
	"context"
	"net/http"
	"time"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/providers/transport"

	"charm.land/fantasy"
	fantasygoogle "charm.land/fantasy/providers/google"
	"google.golang.org/genai"
)

// Client wraps the Google Gemini API client with Fantasy for chat.
type Client struct {
	*genai.Client

	fantasy fantasy.Provider
}

// New creates a Google Gemini client with the given URL (optional), token, and response timeout.
func New(url, token string, timeout time.Duration) (*Client, error) {
	tr := transport.New(transport.WithTimeout(timeout))
	httpClient := &http.Client{Transport: tr}

	config := &genai.ClientConfig{
		APIKey:     token,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
	}

	if url != "" {
		config.HTTPOptions = genai.HTTPOptions{BaseURL: url}
	}

	client, err := genai.NewClient(context.Background(), config)
	if err != nil {
		return nil, err
	}

	// Fantasy provider for Chat (streaming).
	fpOpts := []fantasygoogle.Option{
		fantasygoogle.WithHTTPClient(httpClient),
	}

	if token != "" {
		fpOpts = append(fpOpts, fantasygoogle.WithGeminiAPIKey(token))
	}

	if url != "" {
		fpOpts = append(fpOpts, fantasygoogle.WithBaseURL(url))
	}

	fp, err := fantasygoogle.New(fpOpts...)
	if err != nil {
		debug.Log("[google] fantasy provider init: %v", err)
	}

	debug.Log("[google] initialized (url=%s)", url)

	return &Client{
		Client:  client,
		fantasy: fp,
	}, nil
}
