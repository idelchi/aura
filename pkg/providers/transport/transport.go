// Package transport provides HTTP transport with Bearer token authentication.
package transport

import (
	"crypto/tls"
	"net/http"
	"time"
)

// Option configures a Transport.
type Option func(*Transport)

// Transport is an http.RoundTripper that adds Bearer token authentication.
type Transport struct {
	// Base is the underlying transport.
	Base *http.Transport
	// Token is the Bearer token for authentication.
	Token string
}

// WithToken sets the Bearer token for authentication.
func WithToken(token string) Option {
	return func(t *Transport) {
		t.Token = token
	}
}

// WithTimeout sets the ResponseHeaderTimeout on the underlying transport.
// This limits how long the client waits for response headers after sending a request,
// without affecting long-running streaming responses.
func WithTimeout(d time.Duration) Option {
	return func(t *Transport) {
		t.Base.ResponseHeaderTimeout = d
	}
}

// New creates a Transport with the given options.
func New(opts ...Option) *Transport {
	t := &Transport{
		Base: &http.Transport{
			TLSClientConfig: &tls.Config{
				CurvePreferences: []tls.CurveID{
					tls.X25519,
					tls.CurveP256,
				},
			},
		},
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// RoundTrip implements http.RoundTripper by adding Bearer token authentication.
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		return http.DefaultTransport.RoundTrip(r)
	}

	req := r.Clone(r.Context())

	if t.Token != "" {
		req.Header.Set("Authorization", "Bearer "+t.Token)
	}

	return base.RoundTrip(req)
}
