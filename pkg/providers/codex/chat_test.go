package codex

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/thinking"

	fantasyopenai "charm.land/fantasy/providers/openai"
)

type endpointProbe func(*http.Request) (*http.Response, error)

func (f endpointProbe) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestChatUsesResponsesForNewModel(t *testing.T) {
	t.Parallel()
	client := New("https://codex.example", "unused", "", time.Second)
	seen := false
	client.inner.HTTPClient.Transport = endpointProbe(func(r *http.Request) (*http.Response, error) {
		seen = true
		if r.URL.Path != "/responses" {
			t.Errorf("Codex endpoint = %q, want /responses", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"wire inspected","type":"invalid_request_error"}}`)),
			Request:    r,
		}, nil
	})
	_, _, err := client.Chat(t.Context(), request.Request{
		Model:    model.Model{Name: "future-codex-model"},
		Messages: message.Messages{{Role: roles.User, Content: "Hello"}},
	}, nil)
	if !seen || err == nil || !strings.Contains(err.Error(), "wire inspected") {
		t.Fatalf("Codex request did not reach transport: %v", err)
	}
}

func TestReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		think      thinking.Value
		wantEffort fantasyopenai.ReasoningEffort
		wantOK     bool
		wantErr    bool
	}{
		{name: "auto omits effort", think: thinking.NewValue(true)},
		{
			name:       "off maps to none",
			think:      thinking.NewValue(false),
			wantEffort: fantasyopenai.ReasoningEffortNone,
			wantOK:     true,
		},
		{
			name:       "minimal maps through",
			think:      thinking.NewValue("minimal"),
			wantEffort: fantasyopenai.ReasoningEffortMinimal,
			wantOK:     true,
		},
		{
			name:       "xhigh maps through",
			think:      thinking.NewValue("xhigh"),
			wantEffort: fantasyopenai.ReasoningEffortXHigh,
			wantOK:     true,
		},
		{name: "max is rejected", think: thinking.NewValue("max"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := reasoningEffort(tt.think.Ptr())
			if tt.wantErr {
				if err == nil {
					t.Fatal("reasoningEffort() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("reasoningEffort() unexpected error: %v", err)
			}

			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}

			if ok && got != tt.wantEffort {
				t.Fatalf("effort = %q, want %q", got, tt.wantEffort)
			}
		})
	}
}
