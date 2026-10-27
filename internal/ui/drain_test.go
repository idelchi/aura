package ui_test

import (
	"testing"
	"time"

	"github.com/idelchi/aura/internal/ui"
)

// pendingPrompts queues an unanswered question, an approval request and a flush.
func pendingPrompts(events chan<- ui.Event) (<-chan string, <-chan ui.ConfirmAction, chan struct{}) {
	ask := make(chan string, 1)
	confirm := make(chan ui.ConfirmAction, 1)
	flushed := make(chan struct{})

	events <- ui.AskRequired{Question: "Which?", Options: []ui.AskOption{{Label: "first"}}, Response: ask}
	events <- ui.ToolConfirmRequired{ToolName: "Bash", Response: confirm}
	events <- ui.Flush{Done: flushed}

	return ask, confirm, flushed
}

// assertDeniedAndDismissed fails unless cancellation denied, dismissed and flushed.
func assertDeniedAndDismissed(t *testing.T, ask <-chan string, confirm <-chan ui.ConfirmAction, flushed chan struct{}) {
	t.Helper()

	if got := <-ask; got != "" {
		t.Errorf("question answered %q on cancellation, want dismissed", got)
	}

	if got := <-confirm; got != ui.ConfirmDeny {
		t.Errorf("confirmation answered %v on cancellation, want denied", got)
	}

	select {
	case <-flushed:
	default:
		t.Error("flush not released")
	}
}

func TestDrainEventsNeverApproves(t *testing.T) {
	events := make(chan ui.Event, 10)
	ask, confirm, flushed := pendingPrompts(events)

	ui.DrainEvents(events)

	assertDeniedAndDismissed(t, ask, confirm, flushed)
}

// TestDrainEventsClosedChannel drains what a closed channel still holds, then
// returns: a closed channel is always ready, so receiving without the ok check
// would yield nil events for ever.
func TestDrainEventsClosedChannel(t *testing.T) {
	events := make(chan ui.Event, 10)
	ask, confirm, flushed := pendingPrompts(events)

	close(events)

	drained := make(chan struct{})

	go func() {
		ui.DrainEvents(events)
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(10 * time.Second):
		t.Fatal("DrainEvents did not return for a closed channel")
	}

	assertDeniedAndDismissed(t, ask, confirm, flushed)
}
