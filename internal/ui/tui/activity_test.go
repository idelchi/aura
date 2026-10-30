package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"

	tea "charm.land/bubbletea/v2"
)

// activityHarness drives a real Model through Update with a controlled clock.
type activityHarness struct {
	t      *testing.T
	m      Model
	now    time.Time
	cancel chan struct{}
}

func newActivityHarness(t *testing.T) *activityHarness {
	t.Helper()

	h := &activityHarness{
		t:      t,
		now:    time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		cancel: make(chan struct{}, 8),
	}

	h.m = NewModel(ui.Status{Agent: "test"}, make(chan ui.UserInput, 8), make(chan ui.UserAction, 8), h.cancel, "", nil, "")
	h.m.clock = func() time.Time { return h.now }
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})

	return h
}

func (h *activityHarness) send(msgs ...tea.Msg) {
	h.t.Helper()

	for _, msg := range msgs {
		next, _ := h.m.Update(msg)
		h.m = next.(Model)
	}
}

func (h *activityHarness) advance(d time.Duration) { h.now = h.now.Add(d) }

func (h *activityHarness) key(code rune) {
	h.send(tea.KeyPressMsg{Code: code})
}

// view returns the rendered screen without styling.
func (h *activityHarness) view() string {
	return ansi.Strip(h.m.View().Content)
}

// row returns the activity row as rendered on screen.
func (h *activityHarness) row() string {
	return ansi.Strip(h.m.renderActivity(h.now))
}

// expectWorking asserts the row animates and names the phase.
func (h *activityHarness) expectWorking(phase string) {
	h.t.Helper()

	if state := h.m.activity.State(); state != ui.ActivityWorking {
		h.t.Errorf("activity state = %v, want working; row %q", state, h.row())
	}

	if row := h.row(); !strings.HasPrefix(row, ansi.Strip(h.m.spinner.View())+phase) {
		h.t.Errorf("row = %q, want spinner and %q", row, phase)
	}
}

// toolCall returns a Bash call in the given state.
func toolCall(id string, state call.State) *call.Call {
	c := &call.Call{ID: id, Name: "Bash"}
	c.SetArgs(map[string]any{"command": "make"})

	switch state {
	case call.Running:
		c.MarkRunning()
	case call.Complete:
		c.Complete("ok", 1)
	}

	return c
}

// toolAdded registers a pending tool call, as the builder does before running it.
func toolAdded(id string) ui.MessagePartAdded {
	return ui.MessagePartAdded{MessageID: "msg-1", Part: part.Part{Type: part.Tool, Call: toolCall(id, call.Pending)}}
}

// toolUpdate moves the tool call at index of the current message to state.
func toolUpdate(index int, id string, state call.State) ui.MessagePartUpdated {
	return ui.MessagePartUpdated{MessageID: "msg-1", PartIndex: index, Part: part.Part{Type: part.Tool, Call: toolCall(id, state)}}
}

func TestActivityRowCoversWholeTurn(t *testing.T) {
	t.Parallel()

	h := newActivityHarness(t)

	if row := h.row(); strings.TrimSpace(row) != "Ready" {
		t.Errorf("idle row = %q, want Ready", row)
	}

	h.send(
		ui.UserMessagesProcessed{Texts: []string{"build it"}},
		ui.SpinnerMessage{Text: "Preparing..."},
		ui.MessageAdded{Message: message.Message{Role: roles.User, Parts: []part.Part{contentPart("build it")}}},
		ui.MessageStarted{MessageID: "msg-1"},
	)
	h.expectWorking("Waiting for model")

	// Hidden thinking: nothing in the history, but the row says what happens.
	h.advance(time.Second)
	h.send(ui.MessagePartAdded{MessageID: "msg-1", Part: thinkingPart("secret plan")})
	h.expectWorking("Thinking")

	if strings.Contains(h.view(), "secret plan") {
		t.Errorf("hidden thinking leaked into the screen")
	}

	// Silence after output (e.g. tool arguments being generated) is not presented as thinking.
	h.advance(5 * time.Second)
	h.expectWorking("Generating response")

	if row := h.row(); !strings.Contains(row, "last update 5s ago") {
		t.Errorf("quiet row = %q, want the time since the last update", row)
	}

	h.send(
		toolAdded("1"),
		toolUpdate(1, "1", call.Running),
	)
	h.expectWorking("Running Bash")

	// Typing and queueing during the turn are untouched.
	h.m.textarea.SetValue("next question")
	h.key(tea.KeyEnter)

	if len(h.m.pendingMessages) != 1 {
		t.Errorf("pending = %q, want the queued message", h.m.pendingMessages)
	}

	h.expectWorking("Running Bash")

	// Between the tool result and the next model output nothing streams.
	h.send(toolUpdate(1, "1", call.Complete))
	h.expectWorking("Waiting for model")

	// A mid-turn finalize (compaction) keeps the turn going.
	h.send(ui.MessageFinalized{Message: message.Message{ID: "msg-1", Role: roles.Assistant}})
	h.expectWorking("Working")

	h.send(ui.SpinnerMessage{Text: "Compacting context..."})
	h.expectWorking("Compacting context")

	h.send(
		ui.MessageStarted{MessageID: "msg-2"},
		ui.MessagePartAdded{MessageID: "msg-2", Part: contentPart("Built.")},
	)
	h.expectWorking("Writing response")

	h.advance(4 * time.Second)
	h.send(
		ui.MessageFinalized{Message: message.Message{ID: "msg-2", Role: roles.Assistant, Parts: []part.Part{contentPart("Built.")}}},
		ui.AssistantDone{},
	)

	if row := h.row(); row != "✓ Done in 10s" {
		t.Errorf("row after the turn = %q, want the outcome", row)
	}
}

func TestActivityRowAwaitingUser(t *testing.T) {
	t.Parallel()

	h := newActivityHarness(t)
	h.send(ui.UserMessagesProcessed{}, ui.MessageStarted{MessageID: "msg-1"}, toolAdded("1"), toolUpdate(0, "1", call.Running))

	response := make(chan ui.ConfirmAction, 1)
	h.send(ui.ToolConfirmRequired{ToolName: "Bash", Detail: "make", Pattern: "Bash:make*", Response: response})

	if row := h.row(); !strings.HasPrefix(row, "? Waiting for your confirmation") {
		t.Errorf("confirmation row = %q", row)
	}

	// Enter selects the picker's first item: allow once.
	h.key(tea.KeyEnter)

	if got := <-response; got != ui.ConfirmAllow {
		t.Errorf("confirmation answered %v, want allow", got)
	}

	h.expectWorking("Running Bash")

	answer := make(chan string, 1)
	h.send(ui.AskRequired{Question: "Which target?", Response: answer})

	if row := h.row(); !strings.HasPrefix(row, "? Waiting for your answer") {
		t.Errorf("question row = %q", row)
	}

	h.m.textarea.SetValue("all")
	h.key(tea.KeyEnter)

	if got := <-answer; got != "all" {
		t.Errorf("question answered %q, want all", got)
	}

	h.expectWorking("Running Bash")
}

func TestActivityRowOutcomes(t *testing.T) {
	t.Parallel()

	t.Run("failed", func(t *testing.T) {
		t.Parallel()

		h := newActivityHarness(t)
		h.send(ui.UserMessagesProcessed{}, ui.MessageStarted{})
		h.advance(3 * time.Second)
		h.send(ui.AssistantDone{Error: errors.New("provider unavailable")})

		if row := h.row(); row != "✗ Failed after 3s" {
			t.Errorf("row = %q", row)
		}
	})

	t.Run("cancelled with escape", func(t *testing.T) {
		t.Parallel()

		h := newActivityHarness(t)
		h.send(ui.UserMessagesProcessed{}, ui.MessageStarted{})
		h.key(tea.KeyEscape)

		select {
		case <-h.cancel:
		case <-time.After(10 * time.Second):
			t.Fatal("escape did not request cancellation")
		}

		h.expectWorking("Cancelling")

		h.advance(2 * time.Second)
		h.send(ui.AssistantDone{Error: context.Canceled, Cancelled: true})

		if row := h.row(); row != "■ Cancelled after 2s" {
			t.Errorf("row = %q", row)
		}
	})

	t.Run("background job outside a turn", func(t *testing.T) {
		t.Parallel()

		h := newActivityHarness(t)
		h.send(ui.SpinnerMessage{Text: "Compacting..."})
		h.expectWorking("Compacting")

		h.send(ui.SpinnerMessage{})

		if row := h.row(); strings.TrimSpace(row) != "Ready" {
			t.Errorf("row = %q, want Ready", row)
		}
	})
}

// The activity row is outside the viewport: it stays on screen when the history is
// scrolled, and the layout still fits the terminal.
func TestActivityRowVisibleInScrollback(t *testing.T) {
	t.Parallel()

	h := newActivityHarness(t)

	for i := range 40 {
		h.send(ui.MessageAdded{Message: message.Message{
			Role:  roles.User,
			Parts: []part.Part{contentPart(strings.Repeat("line ", 5) + string(rune('a'+i%26)))},
		}})
	}

	h.send(ui.UserMessagesProcessed{}, ui.MessageStarted{MessageID: "msg-1"}, toolAdded("1"), toolUpdate(0, "1", call.Running))
	h.m.viewport.GotoTop()

	if h.m.viewport.AtBottom() {
		t.Fatal("history does not scroll; the test needs more content")
	}

	// The view ends with a newline; the row after it must fit too, as before the activity row existed.
	lines := strings.Split(h.view(), "\n")
	if len(lines) > h.m.height {
		t.Errorf("view occupies %d rows, terminal has %d", len(lines), h.m.height)
	}

	row := h.m.viewport.Height()
	if row >= len(lines) || !strings.Contains(lines[row], "Running Bash") {
		t.Errorf("line %d below the viewport = %q, want the activity row", row, lines[min(row, len(lines)-1)])
	}
}

func TestDisplayHintsChangedRerendersHistory(t *testing.T) {
	t.Parallel()

	h := newActivityHarness(t)
	h.send(
		ui.UserMessagesProcessed{},
		ui.MessageStarted{MessageID: "msg-1"},
		ui.MessagePartAdded{MessageID: "msg-1", Part: thinkingPart("weighing options")},
	)

	if strings.Contains(h.view(), "weighing options") {
		t.Fatal("thinking shown while hidden")
	}

	h.send(ui.DisplayHintsChanged{Hints: ui.DisplayHints{Verbose: true}})

	if !strings.Contains(h.view(), "weighing options") {
		t.Errorf("thinking still hidden after the backend enabled it:\n%s", h.view())
	}

	h.send(ui.DisplayHintsChanged{Hints: ui.DisplayHints{}})

	if strings.Contains(h.view(), "weighing options") {
		t.Errorf("thinking still shown after the backend disabled it")
	}
}
