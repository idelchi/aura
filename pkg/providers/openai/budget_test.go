package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// Exercise both SDK paths and sequential requests on one client, including zero
// and an omitted budget, so request-local extensions cannot leak into later calls.
func TestThinkingBudgetWire(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"route/local-model", "gpt-5-mini"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var expected *int
			seen := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantPath := "/v1/chat/completions"
				if name == "gpt-5-mini" {
					wantPath = "/v1/responses"
				}
				if r.URL.Path != wantPath {
					t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
				}
				if r.Header.Get("X-Test-Header") != "configured" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("configured header or authentication missing")
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				raw, present := body["thinking_budget_tokens"]
				if present != (expected != nil) {
					t.Errorf("budget presence = %v, expected %v", present, expected != nil)
				}
				if present && expected != nil {
					var value int
					if err := json.Unmarshal(raw, &value); err != nil || value != *expected {
						t.Errorf("budget = %s, want %d", raw, *expected)
					}
				}
				outputCap := false
				for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
					if raw, ok := body[key]; ok && string(raw) == "8192" {
						outputCap = true
					}
				}
				if !outputCap {
					t.Error("output cap lost while applying thinking budget")
				}
				seen <- struct{}{}
				// Serialization is the contract under test; stop before inference.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"error":{"message":"wire inspected","type":"invalid_request_error"}}`)
			}))
			defer server.Close()
			client := New(server.URL+"/v1", "test-key", time.Second, map[string]string{"X-Test-Header": "configured"})
			for _, budget := range []*int{new(2048), nil, new(0), new(4096), nil} {
				expected = budget
				_, _, err := client.Chat(t.Context(), request.Request{
					Model:      model.Model{Name: name},
					Messages:   message.Messages{{Role: roles.User, Content: "Reply OK."}},
					Generation: &generation.Generation{ThinkBudget: budget, MaxOutputTokens: new(8192)},
				}, nil)
				if err == nil {
					t.Fatal("expected the probe server's rejection")
				}
				select {
				case <-seen:
				default:
					t.Fatal("request did not reach probe server")
				}
			}
		})
	}
}
