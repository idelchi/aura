package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// TestGatewayThinkingRequest checks the wire request, not just provider options.
func TestGatewayThinkingRequest(t *testing.T) {
	t.Parallel()

	for _, effort := range []string{"low", "medium", "high"} {
		t.Run(effort, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/models":
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"data":[{"id":"a9/gpt-oss:20b","object":"model"}]}`)
				case "/v1/chat/completions":
					var body struct {
						Model           string `json:"model"`
						ReasoningEffort string `json:"reasoning_effort"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.Model != "a9/gpt-oss:20b" || body.ReasoningEffort != effort {
						t.Errorf("gateway request lost model or effort: %+v", body)
					}
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprint(w, "data: "+`{"id":"completion-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"}}]}`+"\n\n")
					_, _ = fmt.Fprint(w, "data: "+`{"id":"completion-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			client := New(server.URL+"/v1", "test-key", time.Second)
			m, err := client.Model(t.Context(), "a9/gpt-oss:20b")
			if err != nil {
				t.Fatal(err)
			}
			think, err := m.NormalizeThinking(thinking.NewValue(effort), false)
			if err != nil {
				t.Fatal(err)
			}
			msg, _, err := client.Chat(t.Context(), request.Request{
				Model:    m,
				Think:    think.Ptr(),
				Messages: message.Messages{{Role: roles.User, Content: "Reply OK."}},
			}, nil)
			if err != nil || msg.Content != "OK" {
				t.Fatalf("gateway response = %q, error = %v", msg.Content, err)
			}
		})
	}
}
