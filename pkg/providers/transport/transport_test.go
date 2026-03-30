package transport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/providers/transport"
)

// echoHeadersHandler returns an HTTP handler that responds 200 OK and records
// the incoming request for inspection via the pointer it closes over.
func echoHeadersHandler(captured *http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Header.Clone()

		w.WriteHeader(http.StatusOK)
	}
}

func TestRoundTripBearerToken(t *testing.T) {
	t.Parallel()

	var got http.Header

	srv := httptest.NewServer(echoHeadersHandler(&got))
	defer srv.Close()

	tr := transport.New(transport.WithToken("test-token"))
	client := &http.Client{Transport: tr}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	want := "Bearer test-token"
	if got.Get("Authorization") != want {
		t.Errorf("Authorization header = %q, want %q", got.Get("Authorization"), want)
	}
}

func TestRoundTripNoToken(t *testing.T) {
	t.Parallel()

	var got http.Header

	srv := httptest.NewServer(echoHeadersHandler(&got))
	defer srv.Close()

	tr := transport.New() // no token
	client := &http.Client{Transport: tr}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if auth := got.Get("Authorization"); auth != "" {
		t.Errorf("Authorization header = %q, want empty", auth)
	}
}

func TestRoundTripHeaderPassthrough(t *testing.T) {
	t.Parallel()

	var got http.Header

	srv := httptest.NewServer(echoHeadersHandler(&got))
	defer srv.Close()

	tr := transport.New(transport.WithToken("tok"))
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("X-Custom-Header", "custom-value")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	cases := []struct {
		header string
		want   string
	}{
		{"X-Custom-Header", "custom-value"},
		{"Accept", "application/json"},
		{"Authorization", "Bearer tok"},
	}
	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			t.Parallel()

			if v := got.Get(tc.header); v != tc.want {
				t.Errorf("header %q = %q, want %q", tc.header, v, tc.want)
			}
		})
	}
}

func TestWithTimeout(t *testing.T) {
	t.Parallel()

	d := 42 * time.Second
	tr := transport.New(transport.WithTimeout(d))

	if tr.Base.ResponseHeaderTimeout != d {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", tr.Base.ResponseHeaderTimeout, d)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	tr := transport.New()

	if tr == nil {
		t.Fatal("New() returned nil")
	}

	if tr.Base == nil {
		t.Fatal("New().Base is nil")
	}

	if tr.Base.TLSClientConfig == nil {
		t.Fatal("New().Base.TLSClientConfig is nil")
	}
}
