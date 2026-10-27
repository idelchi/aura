package script

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/providers"
)

func write(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "script.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestReplayPersistsCursorAcrossProviders(t *testing.T) {
	path := write(t, `{"models":[{"name":"m","context":100}],"steps":[
		{"content":"one"},
		{"calls":[{"name":"Bash","args":{"command":"x"}}]},
		{"error":{"class":"server","status":503,"message":"the engine is not running"}}]}`)

	first, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	msg, _, err := first.Chat(t.Context(), request.Request{}, nil)
	if err != nil || msg.Content != "one" {
		t.Fatalf("step 1 = %+v, %v", msg, err)
	}

	// A new process continues where the previous one stopped.
	second, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	msg, _, err = second.Chat(t.Context(), request.Request{}, nil)
	if err != nil || len(msg.Calls) != 1 || msg.Calls[0].ID != "script-2-1" || msg.Calls[0].Arguments["command"] != "x" {
		t.Fatalf("step 2 = %+v, %v", msg, err)
	}

	_, _, err = second.Chat(t.Context(), request.Request{}, nil)

	var status *providers.StatusError
	if !errors.Is(err, providers.ErrServerError) || !errors.As(err, &status) || status.Status != 503 {
		t.Fatalf("step 3 = %v, want a classified 503", err)
	}

	if _, _, err := second.Chat(t.Context(), request.Request{}, nil); !errors.Is(err, ErrExhausted) {
		t.Fatalf("after the script = %v, want ErrExhausted", err)
	}

	if m, err := second.Model(t.Context(), "m"); err != nil || m.ContextLength != 100 {
		t.Fatalf("Model = %+v, %v", m, err)
	}
}

func TestWaitsHonourCancellation(t *testing.T) {
	provider, err := New(write(t, `{"steps":[{"delay":"1h"},{"content":"x"}]}`))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, _, err := provider.Chat(ctx, request.Request{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Chat(cancelled) = %v", err)
	}
}

func TestInvalidScriptsFailToLoad(t *testing.T) {
	for _, body := range []string{
		`{"steps":[{"error":{"class":"weird"}}]}`,
		`{"steps":[{"delay":"soon"}]}`,
		`{"steps":[{"unknown":1}]}`,
		`{"steps":[{"finish":"truncated"}]}`,
	} {
		if _, err := New(write(t, body)); err == nil {
			t.Errorf("script %s loaded", body)
		}
	}
}

// TestStepsReportFinish replays an explicit finish reason, or the one a complete step implies.
func TestStepsReportFinish(t *testing.T) {
	p, err := New(write(t, `{"steps":[
		{"content":"done"},
		{"calls":[{"name":"Bash","args":{"command":"x"}}]},
		{"content":"partial","finish":"length"},
		{"content":"unsure","finish":"unreported"}]}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []message.Finish{message.FinishStop, message.FinishToolCalls, message.FinishLength, message.FinishUnreported} {
		msg, _, err := p.Chat(context.Background(), request.Request{}, nil)
		if err != nil || msg.Finish != want {
			t.Fatalf("finish = %q (%v), want %q", msg.Finish, err, want)
		}
	}
}
