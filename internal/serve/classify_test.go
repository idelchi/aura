package serve_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/packages/ssestream"

	"github.com/idelchi/aura/internal/serve"
	"github.com/idelchi/aura/pkg/automation"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/llamacpp"
)

func TestClassifyLlamaCPPStreamErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		payload   string
		class     error
		code      automation.Code
		retryable bool
	}{
		{
			name:      "engine stopped after streaming began",
			payload:   `{"error":{"type":"server_error","message":"the engine stopped unexpectedly (exit code -6); the next request restarts it"}}`,
			class:     providers.ErrServerError,
			code:      automation.CodeProviderServer,
			retryable: true,
		},
		{"server type without diagnostic", `{"error":{"type":"server_error"}}`, providers.ErrServerError, automation.CodeProviderServer, true},
		{"unavailable", `{"error":{"code":503,"type":"unavailable_error","message":"not running"}}`, providers.ErrServerError, automation.CodeProviderServer, true},
		{"rate limit", `{"error":{"type":"rate_limit_error","code":"queue_wait_timeout","message":"queue expired"}}`, providers.ErrRateLimit, automation.CodeProviderRateLimit, true},
		{"authentication", `{"error":{"type":"authentication_error","message":"server_error is not an API key"}}`, providers.ErrAuth, automation.CodeProviderAuth, false},
		{"input", `{"error":{"type":"invalid_request_error","message":"server_error is not a valid parameter"}}`, nil, automation.CodeInternal, false},
		{"context size", `{"error":{"type":"exceed_context_size_error","message":"context full"}}`, providers.ErrContextExhausted, automation.CodeContextExhausted, false},
		{"unknown", `{"error":{"type":"unknown_error","message":"server_error"}}`, nil, automation.CodeInternal, false},
		{"text error", `{"error":"server_error"}`, nil, automation.CodeInternal, false},
		{"invalid type", `{"error":{"type":500,"message":"server_error"}}`, nil, automation.CodeInternal, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", tc.payload)
			}))
			defer server.Close()

			client := llamacpp.New(server.URL, "", time.Second)
			msg, _, err := client.Chat(t.Context(), request.Request{
				Model:    model.Model{Name: "m"},
				Messages: message.Messages{{Role: roles.User, Content: "hi"}},
			}, nil)
			if err == nil || msg.Content != "" {
				t.Fatalf("stream failure returned content %q, error %v", msg.Content, err)
			}

			if tc.class != nil && !errors.Is(err, tc.class) {
				t.Errorf("error %v is not %v", err, tc.class)
			}

			original, ok := errors.AsType[*ssestream.StreamError](err)
			if !ok || strings.TrimSpace(string(original.Event.Data)) != tc.payload {
				t.Fatalf("original SDK stream event lost: %v", err)
			}

			failure := serve.Classify(fmt.Errorf("provider request: %w", err))
			if failure.Code != tc.code || failure.Retryable != tc.retryable {
				t.Errorf("classification = %+v, want code %s, retryable %v", failure, tc.code, tc.retryable)
			}
			if failure.Details.Status != 0 {
				t.Errorf("stream error invented an HTTP failure status: %+v", failure.Details)
			}
		})
	}
}
