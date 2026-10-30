package ui_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// activityClock drives an Activity through events at controlled times.
type activityClock struct {
	t   *testing.T
	a   ui.Activity
	now time.Time
}

func newActivityClock(t *testing.T) *activityClock {
	t.Helper()

	return &activityClock{t: t, now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *activityClock) after(d time.Duration, events ...ui.Event) {
	c.now = c.now.Add(d)

	for _, e := range events {
		c.a.Observe(e, c.now)
	}
}

func (c *activityClock) expect(state ui.ActivityState, display string) {
	c.t.Helper()

	if got := c.a.State(); got != state {
		c.t.Errorf("state = %v, want %v (display %q)", got, state, c.a.Display(c.now))
	}

	if got := c.a.Display(c.now); got != display {
		c.t.Errorf("display = %q, want %q", got, display)
	}
}

func toolEvent(id, name string, state call.State) ui.MessagePartUpdated {
	return ui.MessagePartUpdated{Part: part.Part{Type: part.Tool, Call: &call.Call{ID: id, Name: name, State: state}}}
}

func TestActivityStaysWorkingForWholeTurn(t *testing.T) {
	t.Parallel()

	c := newActivityClock(t)
	c.expect(ui.ActivityReady, "Ready")

	c.after(0, ui.UserMessagesProcessed{Texts: []string{"hi"}}, ui.SpinnerMessage{Text: "Preparing..."})
	c.expect(ui.ActivityWorking, "Preparing · 0s")

	c.after(time.Second, ui.MessageStarted{MessageID: "msg-1"})
	c.expect(ui.ActivityWorking, "Waiting for model · 1s")

	// Hidden or not, thinking deltas are model output.
	c.after(time.Second, ui.MessagePartAdded{Part: part.Part{Type: part.Thinking, Text: "plan"}})
	c.expect(ui.ActivityWorking, "Thinking · 2s")

	// Output stops: the backend does not say what the model does now (e.g. tool arguments).
	c.after(4 * time.Second)
	c.expect(ui.ActivityWorking, "Generating response · 6s · last update 4s ago")

	c.after(0, ui.MessagePartAdded{Part: part.Part{Type: part.Tool, Call: &call.Call{ID: "1", Name: "Bash", State: call.Pending}}})
	c.expect(ui.ActivityWorking, "Preparing tools · 6s")

	c.after(0, toolEvent("1", "Bash", call.Running))
	c.expect(ui.ActivityWorking, "Running Bash · 6s")

	c.after(10*time.Second, ui.ToolOutputDelta{ToolName: "Bash", Line: "building"})
	c.after(5 * time.Second)
	c.expect(ui.ActivityWorking, "Running Bash · 21s · last update 5s ago")

	// Between iterations nothing streams; still working.
	c.after(0, toolEvent("1", "Bash", call.Complete))
	c.expect(ui.ActivityWorking, "Waiting for model · 21s")

	c.after(time.Second, ui.MessagePartAdded{Part: part.Part{Type: part.Content, Text: "Done"}})
	c.expect(ui.ActivityWorking, "Writing response · 22s")

	// A mid-turn finalize (compaction, retry) is not the end of the turn.
	c.after(0, ui.MessageFinalized{}, ui.SpinnerMessage{Text: "Compacting context..."})
	c.expect(ui.ActivityWorking, "Compacting context · 22s")

	c.after(0, ui.SpinnerMessage{})
	c.expect(ui.ActivityWorking, "Working · 22s")

	c.after(2*time.Second, ui.MessageStarted{MessageID: "msg-2"}, ui.MessageFinalized{}, ui.AssistantDone{})
	c.expect(ui.ActivityDone, "Done in 24s")

	// The next turn starts fresh.
	c.after(time.Minute, ui.UserMessagesProcessed{Texts: []string{"again"}})
	c.expect(ui.ActivityWorking, "Waiting for model · 0s")
}

func TestActivityParallelTools(t *testing.T) {
	t.Parallel()

	c := newActivityClock(t)
	c.after(0, ui.UserMessagesProcessed{}, toolEvent("a", "Read", call.Running), toolEvent("b", "Rg", call.Running))
	c.expect(ui.ActivityWorking, "Running 2 tools · 0s")

	c.after(0, toolEvent("a", "Read", call.Complete))
	c.expect(ui.ActivityWorking, "Running Rg · 0s")

	c.after(0, toolEvent("b", "Rg", call.Error))
	c.expect(ui.ActivityWorking, "Waiting for model · 0s")
}

func TestActivityAwaitingUser(t *testing.T) {
	t.Parallel()

	c := newActivityClock(t)
	c.after(0, ui.UserMessagesProcessed{}, toolEvent("1", "Bash", call.Running))

	c.after(2 * time.Second)
	c.a.AwaitUser(ui.AwaitingConfirmation, c.now)
	c.expect(ui.ActivityAwaitingUser, "Waiting for your confirmation · 2s")

	// The user's own time is not reported as a silent backend.
	c.after(time.Minute)
	c.a.AwaitUser(ui.AwaitingNothing, c.now)
	c.expect(ui.ActivityWorking, "Running Bash · 1m2s")

	c.a.AwaitUser(ui.AwaitingAnswer, c.now)
	c.expect(ui.ActivityAwaitingUser, "Waiting for your answer · 1m2s")
}

func TestActivityOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cancel  bool
		done    ui.AssistantDone
		state   ui.ActivityState
		display string
	}{
		{name: "done", done: ui.AssistantDone{}, state: ui.ActivityDone, display: "Done in 5s"},
		{name: "failed", done: ui.AssistantDone{Error: errors.New("boom")}, state: ui.ActivityFailed, display: "Failed after 5s"},
		{
			name:    "cancelled",
			cancel:  true,
			done:    ui.AssistantDone{Error: context.Canceled, Cancelled: true},
			state:   ui.ActivityCancelled,
			display: "Cancelled after 5s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newActivityClock(t)
			c.after(0, ui.UserMessagesProcessed{}, ui.MessageStarted{})

			if tt.cancel {
				c.a.RequestCancel()
				c.expect(ui.ActivityWorking, "Cancelling · 0s")
			}

			c.after(5*time.Second, tt.done)
			c.expect(tt.state, tt.display)
		})
	}
}

func TestActivityBackgroundJob(t *testing.T) {
	t.Parallel()

	c := newActivityClock(t)

	// A slash command outside a turn, cleared explicitly.
	c.after(0, ui.SpinnerMessage{Text: "Compacting..."})
	c.expect(ui.ActivityWorking, "Compacting")

	c.after(4 * time.Second)
	c.expect(ui.ActivityWorking, "Compacting · last update 4s ago")

	c.after(0, ui.SpinnerMessage{})
	c.expect(ui.ActivityReady, "Ready")

	// A slash command that never clears its text ends with SlashCommandHandled.
	c.after(0, ui.SpinnerMessage{Text: "Fetching models..."}, ui.SlashCommandHandled{Text: "/model"})
	c.expect(ui.ActivityReady, "Ready")

	// Cancelling outside a turn changes nothing.
	c.a.RequestCancel()
	c.expect(ui.ActivityReady, "Ready")
}
