package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/part"
)

type transcriptSink struct {
	writes chan string
	err    error
}

func (s transcriptSink) Write(b []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.writes <- string(b)
	return len(b), nil
}

func TestTranscriptFlushesWhileTurnStillRunning(t *testing.T) {
	sink := transcriptSink{writes: make(chan string, 8)}
	u := New(ui.Status{}, "", nil, sink)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- u.bridgeEvents(ctx, cancel) }()
	u.events <- ui.MessageStarted{MessageID: "a"}
	u.events <- ui.MessagePartAdded{MessageID: "a", Part: part.Part{Type: part.Thinking, Text: "Still thinking."}}
	select {
	case text := <-sink.writes:
		if !strings.Contains(text, "Still thinking.") {
			t.Errorf("missing streamed thinking: %q", text)
		}
	case <-time.After(3 * time.Second):
		t.Error("output log waited for turn completion")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTranscriptWriteFailureCancelsAndReleasesQueuedFlush(t *testing.T) {
	want := errors.New("disk full")
	u := New(ui.Status{}, "", nil, transcriptSink{err: want})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	flush := make(chan struct{})
	u.events <- ui.CommandResult{Message: "help"}
	u.events <- ui.Flush{Done: flush}
	if err := u.bridgeEvents(ctx, cancel); !errors.Is(err, want) {
		t.Fatalf("lost output failure: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("failed output did not stop the UI")
	}
	select {
	case <-flush:
	default:
		t.Fatal("queued flush waiter was stranded")
	}
}
