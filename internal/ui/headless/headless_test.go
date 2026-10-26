package headless_test

import (
	"context"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/internal/ui/headless"
)

func TestHeadlessNeverApproves(t *testing.T) {
	h := headless.New()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- h.Run(ctx) }()

	// While running: nobody can approve a non-interactive run.
	confirm := make(chan ui.ConfirmAction, 1)
	h.Events() <- ui.ToolConfirmRequired{ToolName: "Bash", Response: confirm}

	select {
	case got := <-confirm:
		if got != ui.ConfirmDeny {
			t.Fatalf("headless answered %v, want denied", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("headless did not answer")
	}

	ask := make(chan string, 1)
	h.Events() <- ui.AskRequired{Question: "Which?", Options: []ui.AskOption{{Label: "first"}}, Response: ask}

	if got := <-ask; got != "" {
		t.Fatalf("headless answered %q, want dismissed", got)
	}

	cancel()
	<-done

	// After cancellation: queued prompts are drained the same way.
	late := make(chan ui.ConfirmAction, 1)
	h.Events() <- ui.ToolConfirmRequired{ToolName: "Bash", Response: late}

	cancelled, stop := context.WithCancel(t.Context())
	stop()
	h.Run(cancelled)

	if got := <-late; got != ui.ConfirmDeny {
		t.Fatalf("drained confirmation answered %v, want denied", got)
	}
}
