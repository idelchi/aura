package ollama_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/ollama"
)

// TestEstimate exercises the actual SDK request and provider error classification.
func TestEstimate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		status  int
		body    any
		want    int
		wantErr error
	}{
		{"success", 200, api.ChatResponse{Done: true, Metrics: api.Metrics{PromptEvalCount: 42}}, 42, nil},
		{"legacy overflow", 400, map[string]string{"error": "input length exceeds context length"}, 8192, providers.ErrContextExhausted},
		{"current overflow", 400, map[string]string{"error": "exceed_context_size_error"}, 8192, providers.ErrContextExhausted},
		{"stream overflow", 200, map[string]string{"error": "exceed_context_size_error"}, 8192, providers.ErrContextExhausted},
		{"authentication", 401, map[string]string{"error": "unauthorized"}, 0, providers.ErrAuth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var got api.ChatRequest
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if r.URL.Path != "/api/chat" || got.Model != "test" || len(got.Messages) != 1 || got.Messages[0].Content != "content" {
					t.Errorf("unexpected request: %s %+v", r.URL.Path, got)
				}
				if got.Think == nil || got.Think.Value != false || got.Stream == nil || *got.Stream ||
					got.Truncate == nil || *got.Truncate || got.Shift == nil || *got.Shift ||
					got.Options["num_predict"] != float64(1) || got.Options["num_ctx"] != float64(8192) {
					t.Errorf("estimation must be bounded and must not truncate: %+v", got)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if err := json.NewEncoder(w).Encode(tc.body); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client, err := ollama.New(server.URL, "", 0, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.Estimate(t.Context(), request.Request{Model: model.Model{Name: "test", ContextLength: 8192}}, "content")
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Errorf("Estimate = %d, %v; want %d, %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
