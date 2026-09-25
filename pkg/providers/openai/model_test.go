package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/thinking"
)

// TestModelFromCatalog covers gateway routing IDs without a per-model endpoint.
func TestModelFromCatalog(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected metadata path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "gpt-4o-mini", "object": "model"},
				{"id": "openai/gpt-4o-mini", "object": "model"},
				{"id": "jetson/gpt-4o-mini", "object": "model"},
				{"id": "a9/gpt-oss:20b", "object": "model"},
			},
		})
	}))
	defer server.Close()

	client := New(server.URL+"/v1", "test-key", time.Second)
	for _, name := range []string{"gpt-4o-mini", "openai/gpt-4o-mini"} {
		m, err := client.Model(t.Context(), name)
		if err != nil {
			t.Fatal(err)
		}
		if m.Name != name || m.ContextLength == 0 || !m.Capabilities.Vision() || !m.CapabilitiesKnown {
			t.Fatalf("routing ID or enrichment lost: %+v", m)
		}
	}

	for _, name := range []string{"jetson/gpt-4o-mini", "a9/gpt-oss:20b"} {
		m, err := client.Model(t.Context(), name)
		if err != nil || m.Capabilities.Vision() || m.CapabilitiesKnown {
			t.Fatalf("other provider must not inherit OpenAI metadata: %+v, %v", m, err)
		}
		got, err := m.NormalizeThinking(thinking.NewValue("medium"), false)
		if err != nil || got.Value != "medium" {
			t.Fatalf("missing gateway metadata rejected explicit thinking: %v, %v", got, err)
		}
	}
	if _, err := client.Model(t.Context(), "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing model error: %v", err)
	}
}

// TestModelCatalogFailure ensures an unavailable catalog cannot invent a model.
func TestModelCatalogFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid token","type":"authentication_error"}}`))
	}))
	defer server.Close()

	client := New(server.URL+"/v1", "test-key", time.Second)
	if _, err := client.Model(t.Context(), "openai/gpt-4o-mini"); err == nil {
		t.Fatal("expected catalog authentication error")
	}
}
