package anthropic

import (
	"net/http"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/providers/transport"

	"charm.land/fantasy"
	fanthropic "charm.land/fantasy/providers/anthropic"
)

// Client wraps the Anthropic API client with Fantasy for chat.
type Client struct {
	sdk.Client

	Fantasy fantasy.Provider
}

// New creates an Anthropic client with the given URL (optional), token, and response timeout.
func New(url, token string, timeout time.Duration) *Client {
	// Transport only for timeout — Anthropic uses X-Api-Key header (handled by SDK),
	// NOT Bearer token. Do NOT add transport.WithToken here.
	opts := []transport.Option{transport.WithTimeout(timeout)}
	httpClient := &http.Client{Transport: transport.New(opts...)}

	sdkOpts := []option.RequestOption{
		option.WithHTTPClient(httpClient),
	}

	if url != "" {
		sdkOpts = append(sdkOpts, option.WithBaseURL(url))
	}

	if token != "" {
		sdkOpts = append(sdkOpts, option.WithAPIKey(token))
	}

	// Fantasy provider for Chat (streaming).
	fpOpts := []fanthropic.Option{
		fanthropic.WithHTTPClient(httpClient),
	}

	if url != "" {
		fpOpts = append(fpOpts, fanthropic.WithBaseURL(url))
	}

	if token != "" {
		fpOpts = append(fpOpts, fanthropic.WithAPIKey(token))
	}

	fp, err := fanthropic.New(fpOpts...)
	if err != nil {
		debug.Log("[anthropic] fantasy provider init: %v", err)
	}

	debug.Log("[anthropic] initialized (url=%s)", url)

	return &Client{
		Client:  sdk.NewClient(sdkOpts...),
		Fantasy: fp,
	}
}
