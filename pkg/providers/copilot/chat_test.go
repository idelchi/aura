package copilot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// Exercise the SDK and transport together: model discovery must determine the
// wire endpoint, and neither SDK may demand a separate provider credential.
func TestChatUsesDiscoveredEndpointAndCopilotAuth(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	for _, tc := range []struct{ model, endpoint string }{
		{"future-copilot-model", "/responses"},
		{"claude-opus-5.5", "/v1/messages"},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			received := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer copilot-test-token" {
					t.Error("request did not use the Copilot token")
				}
				if r.Header.Get("X-Api-Key") != "" {
					t.Error("SDK credential leaked onto the wire")
				}
				if r.URL.Path == "/models" {
					entry := copilotModelEntry{ID: tc.model, SupportedEndpoints: []string{tc.endpoint}}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(copilotModelsResponse{Data: []copilotModelEntry{entry}})
					return
				}
				received <- r.URL.Path
				// A downstream error proves SDK authentication and routing reached
				// the server without needing a synthetic streaming implementation.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
					"type": "invalid_request_error", "message": "copilot-test-reached",
				}})
			}))
			defer server.Close()
			client := New("github-test-token", time.Second)
			client.transport.token = "copilot-test-token"
			client.transport.expiresAt = time.Now().Add(time.Hour).Unix()
			client.transport.baseURL = server.URL
			model, err := client.Model(t.Context(), tc.model)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = client.Chat(t.Context(), request.Request{
				Model: model, Messages: message.Messages{{Role: roles.User, Content: "Hello"}},
			}, nil)
			if err == nil || !strings.Contains(err.Error(), "copilot-test-reached") {
				t.Fatalf("chat did not reach the server: %v", err)
			}
			select {
			case path := <-received:
				if path != tc.endpoint {
					t.Errorf("endpoint = %q, want %q", path, tc.endpoint)
				}
			default:
				t.Fatal("no chat request received")
			}
		})
	}
}
