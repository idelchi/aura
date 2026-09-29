package llamacpp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

func TestThinkingWireContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		value   any
		enabled *bool
		effort  string
	}{
		{"unset", nil, nil, ""}, {"auto", "auto", nil, ""}, {"on", true, new(true), ""},
		{"off", false, new(false), ""}, {"none", "none", new(false), "none"}, {"high", "high", new(true), "high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				var body struct {
					Model, ReasoningEffort, ReasoningFormat string
					Kwargs                                  struct {
						Enabled *bool `json:"enable_thinking"`
					} `json:"chat_template_kwargs"`
				}
				// Explicit names retain compatibility with underscore-separated API fields.
				var raw map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
					t.Error(err)
				}
				_ = json.Unmarshal(raw["model"], &body.Model)
				_ = json.Unmarshal(raw["reasoning_effort"], &body.ReasoningEffort)
				_ = json.Unmarshal(raw["reasoning_format"], &body.ReasoningFormat)
				_ = json.Unmarshal(raw["chat_template_kwargs"], &body.Kwargs)
				if body.Model != "arbitrary-model" || body.ReasoningEffort != tc.effort || body.ReasoningFormat != "auto" {
					t.Errorf("wrong controls: %+v", body)
				}
				if (body.Kwargs.Enabled == nil) != (tc.enabled == nil) || (tc.enabled != nil && body.Kwargs.Enabled != nil && *tc.enabled != *body.Kwargs.Enabled) {
					t.Errorf("enable_thinking = %v, want %v", body.Kwargs.Enabled, tc.enabled)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "data: "+`{"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"reason","content":"answer"}}]}`+"\n\n")
				_, _ = fmt.Fprint(w, "data: "+`{"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			c := New(server.URL, "", time.Second)
			msg, _, err := c.Chat(t.Context(), request.Request{Model: model.Model{Name: "arbitrary-model"}, Think: thinking.NewValue(tc.value).Ptr(), Messages: message.Messages{{Role: roles.User, Content: "hi"}}}, nil)
			if err != nil || msg.Content != "answer" || msg.Thinking != "reason" {
				t.Fatalf("chat = %+v, %v", msg, err)
			}
		})
	}
}

func TestTemplateProbeDoesNotConfusePreservationWithThinking(t *testing.T) {
	t.Parallel()
	for _, response := range []string{`{"prompt":"unchanged"}`, `{"unrelated":true}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/props" {
					_, _ = fmt.Fprint(w, `{"chat_template":"template","chat_template_caps":{"supports_preserve_reasoning":false}}`)
				} else {
					_, _ = fmt.Fprint(w, response)
				}
			}))
			defer server.Close()
			m, err := New(server.URL, "", time.Second).Model(t.Context(), "unknown")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.NormalizeThinking(thinking.NewValue(true), false); err != nil {
				t.Fatalf("inconclusive metadata rejected thinking: %v", err)
			}
		})
	}
}

func TestToolResultKeepsIDSeparateFromContent(t *testing.T) {
	t.Parallel()
	params := toChatParams(request.Request{
		Model:    model.Model{Name: "arbitrary-model"},
		Messages: message.Messages{{Role: roles.Tool, ToolCallID: "call-123", Content: "the real result"}},
	})
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Messages []struct {
			Role, Content string
			ToolCallID    string `json:"tool_call_id"`
		}
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 1 || body.Messages[0].Role != "tool" || body.Messages[0].Content != "the real result" || body.Messages[0].ToolCallID != "call-123" {
		t.Fatalf("tool result corrupted: %s", data)
	}
}
