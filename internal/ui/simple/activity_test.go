package simple

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// testClock is advanced by the test and read by Run's goroutine.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// activityHarness runs a Simple whose clock and refresh ticks the test controls.
type activityHarness struct {
	*harness

	clock *testClock
	ticks chan time.Time
}

func startActivity(t *testing.T) *activityHarness {
	t.Helper()

	h := &activityHarness{
		harness: prepare(t),
		clock:   &testClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)},
		ticks:   make(chan time.Time),
	}

	h.s.clock = h.clock.Now
	h.s.ticks = h.ticks
	h.run(t.Context())

	t.Cleanup(func() {
		h.cancel()
		h.stop()
	})

	return h
}

// tick delivers one refresh tick and waits until it has been handled.
func (h *activityHarness) tick() {
	h.t.Helper()

	select {
	case h.ticks <- h.clock.Now():
	case <-time.After(watchdog):
		h.t.Fatal("tick not accepted")
	}

	h.flush()
}

// events sends events and waits until they have been rendered.
func (h *activityHarness) events(events ...ui.Event) {
	h.t.Helper()

	for _, e := range events {
		h.send(e)
	}

	h.flush()
}

// expectPrompt asserts the prompt on screen. Fields are read after a flush, which
// orders them after Run's writes; the test owns the ticks, so Run is idle meanwhile.
func (h *activityHarness) expectPrompt(want string) {
	h.t.Helper()

	if got := ansi.Strip(h.s.drawn); got != want {
		h.t.Errorf("prompt = %q, want %q", got, want)
	}
}

func (h *activityHarness) expectLineOpen(want bool) {
	h.t.Helper()

	if h.s.lineOpen != want {
		h.t.Errorf("stdout mid-line = %v, want %v", h.s.lineOpen, want)
	}
}

func toolPart(id string, state call.State) part.Part {
	c := &call.Call{ID: id, Name: "Bash"}
	c.SetArgs(map[string]any{"command": "make"})

	switch state {
	case call.Running:
		c.MarkRunning()
	case call.Complete:
		c.Complete("ok", 1)
	}

	return part.Part{Type: part.Tool, Call: c}
}

func TestSimpleActivityPromptCoversWholeTurn(t *testing.T) {
	h := startActivity(t)
	h.flush()
	h.expectPrompt("> ")

	h.events(
		ui.UserMessagesProcessed{Texts: []string{"build it"}},
		ui.MessageAdded{Message: message.Message{Role: roles.User, Content: "build it"}},
		ui.MessageStarted{MessageID: "msg-1"},
	)
	h.expectPrompt("[Waiting for model · 0s] > ")

	// Hidden thinking prints nothing; the prompt says what happens.
	h.events(ui.MessagePartAdded{MessageID: "msg-1", Part: part.Part{Type: part.Thinking, Text: "plan"}})
	h.expectLineOpen(false)
	h.expectPrompt("[Thinking · 0s] > ")

	// Streamed text leaves the line open: no prompt may be drawn next to it.
	h.events(ui.MessagePartAdded{MessageID: "msg-1", Part: part.Part{Type: part.Content, Text: "Let me check."}})
	h.expectLineOpen(true)
	h.expectPrompt("")

	h.clock.advance(time.Second)
	h.tick()
	h.expectLineOpen(true)

	// The stream goes quiet mid-line (e.g. tool arguments): the line ends so the activity shows.
	h.clock.advance(3 * time.Second)
	h.tick()
	h.expectLineOpen(false)
	h.expectPrompt("[Generating response · 4s · last update 4s ago] > ")

	h.events(
		ui.MessagePartAdded{MessageID: "msg-1", Part: toolPart("1", call.Pending)},
		ui.MessagePartUpdated{MessageID: "msg-1", PartIndex: 2, Part: toolPart("1", call.Running)},
	)
	h.expectPrompt("[Running Bash · 4s] > ")

	h.events(ui.ToolOutputDelta{ToolName: "Bash", Line: "compiling"})
	h.clock.advance(5 * time.Second)
	h.tick()
	h.expectPrompt("[Running Bash · 9s · last update 5s ago] > ")

	// Between the tool result and the next model output nothing streams.
	h.events(ui.MessagePartUpdated{MessageID: "msg-1", PartIndex: 2, Part: toolPart("1", call.Complete)})
	h.expectLineOpen(false)
	h.expectPrompt("[Waiting for model · 9s] > ")

	// A mid-turn finalize keeps the turn going; only AssistantDone ends it.
	h.events(ui.MessageFinalized{Message: message.Message{ID: "msg-1", Role: roles.Assistant}})
	h.expectPrompt("[Working · 9s] > ")

	h.events(ui.MessageStarted{MessageID: "msg-2"}, ui.MessageFinalized{}, ui.AssistantDone{})
	h.expectPrompt("> ")
}

func TestSimpleActivityPromptEndsLineBeforeTools(t *testing.T) {
	h := startActivity(t)

	h.events(
		ui.UserMessagesProcessed{},
		ui.MessageStarted{MessageID: "msg-1"},
		ui.MessagePartAdded{MessageID: "msg-1", Part: part.Part{Type: part.Content, Text: "Running the build"}},
	)
	h.expectLineOpen(true)

	// The tool header must not continue the text line.
	h.events(ui.MessagePartAdded{MessageID: "msg-1", Part: toolPart("1", call.Pending)})
	h.expectLineOpen(false)
	h.expectPrompt("[Preparing tools · 0s] > ")
}

func TestSimpleActivityPromptAwaitsUser(t *testing.T) {
	h := startActivity(t)
	p := newPrompts()

	h.events(
		ui.UserMessagesProcessed{},
		ui.MessageStarted{MessageID: "msg-1"},
		ui.MessagePartAdded{MessageID: "msg-1", Part: toolPart("1", call.Pending)},
		ui.MessagePartUpdated{MessageID: "msg-1", Part: toolPart("1", call.Running)},
		p.confirmEvent(),
	)
	h.expectPrompt("confirm> ")

	if state := h.s.activity.State(); state != ui.ActivityAwaitingUser {
		t.Errorf("activity state = %v, want awaiting the user", state)
	}

	h.line("1")

	if got := <-p.confirm; got != ui.ConfirmAllow {
		t.Fatalf("confirmation answered %v, want allow", got)
	}

	h.flush()
	h.expectPrompt("[Running Bash · 0s] > ")

	h.events(p.askEvent())
	h.expectPrompt("answer> ")

	h.line("1")

	if got := <-p.ask; got != "first" {
		t.Fatalf("question answered %q, want the first option", got)
	}

	h.flush()

	if prompt := ansi.Strip(h.s.drawn); !strings.HasPrefix(prompt, "[Running Bash") {
		t.Errorf("prompt after answering = %q, want the activity again", prompt)
	}

	p.unanswered(t)
}

func TestSimpleActivityPromptOutcomes(t *testing.T) {
	for name, done := range map[string]ui.AssistantDone{
		"cancelled": {Cancelled: true},
		"failed":    {Error: errors.New("provider unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			h := startActivity(t)

			h.events(
				ui.UserMessagesProcessed{},
				ui.MessageStarted{MessageID: "msg-1"},
				ui.MessagePartAdded{MessageID: "msg-1", Part: part.Part{Type: part.Content, Text: "Partial"}},
			)
			h.expectLineOpen(true)

			h.events(done)
			h.expectLineOpen(false)
			h.expectPrompt("> ")
		})
	}
}
