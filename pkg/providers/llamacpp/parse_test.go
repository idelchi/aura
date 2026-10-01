package llamacpp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool"
)

func TestChatClassifiesGeneratedFormatErrors(t *testing.T) {
	t.Parallel()
	const parserFailure = "The model produced output that does not match the expected peg-native format"

	for _, tc := range []struct {
		name    string
		status  int
		message string
		parse   bool
	}{
		{"stream parser failure", http.StatusOK, parserFailure, true},
		{"HTTP parser failure", http.StatusInternalServerError, parserFailure, true},
		{"stream server failure", http.StatusOK, "failed to allocate compute buffer", false},
		{"HTTP server failure", http.StatusInternalServerError, "failed to allocate compute buffer", false},
		{"authentication failure", http.StatusUnauthorized, "invalid API key", false},
		{"missing model", http.StatusNotFound, "model not found", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := json.Marshal(map[string]any{"error": map[string]any{
					"code": http.StatusInternalServerError, "message": tc.message, "type": "server_error",
				}})
				if err != nil {
					t.Error(err)
					return
				}
				if tc.status == http.StatusOK {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprint(w, "data: "+`{"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"partial reasoning"}}]}`+"\n\n")
					_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write(body)
			}))
			defer server.Close()
			client := New(server.URL, "", time.Second)
			client.Client.Client.Options = append(client.Client.Client.Options, option.WithMaxRetries(0))
			_, _, err := client.Chat(t.Context(), request.Request{
				Model:    model.Model{Name: "arbitrary-model"},
				Messages: message.Messages{{Role: roles.User, Content: "Make a plan."}},
			}, nil)
			if err == nil || errors.Is(err, tool.ErrToolCallParse) != tc.parse {
				t.Fatalf("error = %v, want parse classification %v", err, tc.parse)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Errorf("original diagnostic lost: %v", err)
			}
			if tc.status == http.StatusOK {
				if _, ok := errors.AsType[*ssestream.StreamError](err); !ok {
					t.Errorf("original stream error lost: %v", err)
				}
			} else if _, ok := errors.AsType[*openai.Error](err); !ok {
				t.Errorf("original HTTP error lost: %v", err)
			}
		})
	}
}

func TestChatClassifiesMalformedToolArguments(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		arguments string
		parse     bool
	}{
		{"valid object", `{"items":[{"content":"Inspect"},{"content":"Verify"}]}`, false},
		{"empty arguments", "", false},
		{"invalid JSON", `{"items":[}`, true},
		{"wrong JSON type", `["Inspect","Verify"]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				arguments, err := json.Marshal(tc.arguments)
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: "+`{"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"TodoCreate","arguments":%s}}]}}]}`+"\n\n", arguments)
				_, _ = fmt.Fprint(w, "data: "+`{"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			msg, _, err := New(server.URL, "", time.Second).Chat(t.Context(), request.Request{
				Model:    model.Model{Name: "arbitrary-model"},
				Messages: message.Messages{{Role: roles.User, Content: "Make a plan."}},
			}, nil)
			if tc.parse {
				if !errors.Is(err, tool.ErrToolCallParse) {
					t.Fatalf("argument decoding did not trigger parse recovery: %v", err)
				}
				if tc.name == "invalid JSON" {
					if _, ok := errors.AsType[*json.SyntaxError](err); !ok {
						t.Errorf("original JSON error lost: %v", err)
					}
				}
			} else if err != nil || len(msg.Calls) != 1 || msg.Calls[0].Name != "TodoCreate" {
				t.Fatalf("valid tool call changed: %+v, %v", msg, err)
			}
		})
	}
}
