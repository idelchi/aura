package ollama_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/ollama"
)

// streamServer answers /api/chat with the given NDJSON lines.
func streamServer(t *testing.T, lines ...string) *ollama.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")

		for _, line := range lines {
			w.Write([]byte(line + "\n"))
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(server.Close)

	client, err := ollama.New(server.URL, "", 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func chatRequest() request.Request {
	return request.Request{
		Model:    model.Model{Name: "m"},
		Messages: message.New(message.Message{Role: roles.User, Content: "hi"}),
	}
}

func TestChatRequiresTheFinalChunk(t *testing.T) {
	complete := streamServer(t,
		`{"model":"m","message":{"role":"assistant","content":"Hel"},"done":false}`,
		`{"model":"m","message":{"role":"assistant","content":"lo"},"done":true,"eval_count":2}`,
	)

	msg, _, err := complete.Chat(t.Context(), chatRequest(), nil)
	if err != nil || msg.Content != "Hello" {
		t.Fatalf("complete stream = %q, %v", msg.Content, err)
	}

	// The connection ends before the done chunk: a truncated response, not a completed one.
	truncated := streamServer(t, `{"model":"m","message":{"role":"assistant","content":"Hel"},"done":false}`)

	if msg, _, err := truncated.Chat(t.Context(), chatRequest(), nil); !errors.Is(err, providers.ErrNetwork) {
		t.Fatalf("truncated stream = %q, %v; want a network error", msg.Content, err)
	}
}
