package simple

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/ui"
)

// watchdog bounds a wait that must finish; it is never a success condition.
const watchdog = 10 * time.Second

// rounds repeats scenarios whose event, line and cancellation order the
// scheduler chooses, so each run exercises several interleavings.
const rounds = 25

// harness runs one Simple whose readline input is a pipe the test controls.
type harness struct {
	t      *testing.T
	s      *Simple
	input  *io.PipeWriter
	cancel context.CancelFunc
	done   chan error
}

// prepare builds a Simple whose events can be queued before run starts it.
func prepare(t *testing.T) *harness {
	t.Helper()

	reader, writer := io.Pipe()
	s := New(ui.Status{Agent: "test"}, "", os.Stdout, os.Stdout)
	s.stdin = reader

	return &harness{t: t, s: s, input: writer, cancel: func() {}, done: make(chan error, 1)}
}

// run starts Run until ctx (or the harness's own cancel) ends it.
func (h *harness) run(ctx context.Context) {
	ctx, h.cancel = context.WithCancel(ctx)

	go func() { h.done <- h.s.Run(ctx) }()
}

// start prepares and runs a Simple.
func start(t *testing.T, ctx context.Context) *harness {
	t.Helper()

	h := prepare(t)
	h.run(ctx)

	return h
}

// send queues an event exactly as the assistant does.
func (h *harness) send(event ui.Event) {
	h.t.Helper()

	select {
	case h.s.Events() <- event:
	case <-time.After(watchdog):
		h.t.Fatal("event not accepted")
	}
}

// flush returns once every event sent before it has been rendered.
func (h *harness) flush() {
	h.t.Helper()

	done := make(chan struct{})
	h.send(ui.Flush{Done: done})

	select {
	case <-done:
	case <-time.After(watchdog):
		h.t.Fatal("flush not released")
	}
}

// line types one line, as a user pressing Enter.
func (h *harness) line(text string) {
	h.t.Helper()

	if _, err := h.input.Write([]byte(text + "\n")); err != nil {
		h.t.Fatalf("typing %q: %v", text, err)
	}
}

// stop waits for Run to return, then for its readline goroutine to exit.
func (h *harness) stop() error {
	h.t.Helper()

	var err error

	select {
	case err = <-h.done:
	case <-time.After(watchdog):
		h.t.Fatal("simple UI did not stop")
	}

	h.cancel()
	h.input.Close()
	waitForStacks(h.t, "simple.(*Simple).readlineLoop", false)

	return err
}

// waitForStacks waits until some goroutine's stack contains fragment (present) or
// none does (absent). It yields instead of sleeping; the watchdog only bounds failure.
func waitForStacks(t *testing.T, fragment string, present bool) {
	t.Helper()

	deadline := time.Now().Add(watchdog)
	buf := make([]byte, 1<<20)

	for time.Now().Before(deadline) {
		n := runtime.Stack(buf, true)
		if strings.Contains(string(buf[:n]), fragment) == present {
			return
		}

		runtime.Gosched()
	}

	if present {
		t.Fatalf("no goroutine reached %s", fragment)
	}

	t.Fatalf("a goroutine is still in %s", fragment)
}

// prompts are an unanswered question and an approval request, each answerable once.
type prompts struct {
	ask     chan string
	confirm chan ui.ConfirmAction
}

func newPrompts() prompts {
	return prompts{ask: make(chan string, 1), confirm: make(chan ui.ConfirmAction, 1)}
}

func (p prompts) askEvent() ui.AskRequired {
	return ui.AskRequired{Question: "Which?", Options: []ui.AskOption{{Label: "first"}}, Response: p.ask}
}

func (p prompts) confirmEvent() ui.ToolConfirmRequired {
	return ui.ToolConfirmRequired{ToolName: "Bash", Pattern: "Bash:ls*", Response: p.confirm}
}

// unanswered fails if either prompt was answered while it should still wait.
func (p prompts) unanswered(t *testing.T) {
	t.Helper()

	if len(p.ask) != 0 || len(p.confirm) != 0 {
		t.Fatal("a prompt was answered without the user")
	}
}

// dismissed fails unless the question was dismissed and the confirmation denied,
// each exactly once.
func (p prompts) dismissed(t *testing.T) {
	t.Helper()

	select {
	case got := <-p.ask:
		if got != "" {
			t.Errorf("question answered %q at shutdown, want dismissed", got)
		}
	default:
		t.Error("question left blocked at shutdown")
	}

	select {
	case got := <-p.confirm:
		if got != ui.ConfirmDeny {
			t.Errorf("confirmation answered %v at shutdown, want denied", got)
		}
	default:
		t.Error("confirmation left blocked at shutdown")
	}

	if len(p.ask) != 0 || len(p.confirm) != 0 {
		t.Error("a prompt was answered twice")
	}
}

// released fails unless done was closed.
func released(t *testing.T, done chan struct{}) {
	t.Helper()

	select {
	case <-done:
	default:
		t.Error("flush waiter not released at shutdown")
	}
}

// TestSimpleCancelAnswersQueuedPrompts starts Run already cancelled, with a
// question, a confirmation and a flush queued: Run may render some before it sees
// the cancellation and drains the rest; either way none is approved or left blocked.
func TestSimpleCancelAnswersQueuedPrompts(t *testing.T) {
	for range rounds {
		h := prepare(t)
		p := newPrompts()
		flushed := make(chan struct{})

		h.send(p.askEvent())
		h.send(p.confirmEvent())
		h.send(ui.Flush{Done: flushed})

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		h.run(ctx)

		if err := h.stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}

		p.dismissed(t)
		released(t, flushed)
	}
}

// TestSimpleCancelDismissesRenderedPrompts renders a question and a confirmation,
// so both are pending on the prompt, then cancels.
func TestSimpleCancelDismissesRenderedPrompts(t *testing.T) {
	for range rounds {
		h := start(t, t.Context())
		p := newPrompts()

		h.send(p.confirmEvent())
		h.send(p.askEvent())
		h.flush()
		p.unanswered(t)

		h.cancel()

		if err := h.stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}

		p.dismissed(t)
	}
}

// TestSimpleEndOfInputDismissesPrompts ends input with a rendered confirmation and
// a question and flush still queued behind it; Run returns io.EOF without the
// cancellation path, and still answers all of them.
func TestSimpleEndOfInputDismissesPrompts(t *testing.T) {
	for range rounds {
		h := start(t, t.Context())
		p := newPrompts()
		flushed := make(chan struct{})

		h.send(p.confirmEvent())
		h.flush()
		h.send(p.askEvent())
		h.send(ui.Flush{Done: flushed})
		h.input.Close()

		if err := h.stop(); !errors.Is(err, io.EOF) {
			t.Fatalf("Run returned %v, want io.EOF", err)
		}

		p.dismissed(t)
		released(t, flushed)
	}
}

// TestSimpleAnswersInOrdinaryUse keeps the interactive behavior: a rendered
// question and confirmation take the next typed line, Ctrl+C dismisses a
// question, other lines reach the assistant, and shutdown answers nothing again.
func TestSimpleAnswersInOrdinaryUse(t *testing.T) {
	h := start(t, t.Context())
	p := newPrompts()

	h.send(p.askEvent())
	h.flush()
	h.line("1")

	if got := <-p.ask; got != "first" {
		t.Fatalf("question answered %q, want the first option", got)
	}

	h.send(p.confirmEvent())
	h.flush()
	h.line("2")

	if got := <-p.confirm; got != ui.ConfirmAllowSession {
		t.Fatalf("confirmation answered %v, want allowed for the session", got)
	}

	h.send(p.askEvent())
	h.flush()

	if _, err := h.input.Write([]byte{3}); err != nil { // Ctrl+C
		t.Fatal(err)
	}

	if got := <-p.ask; got != "" {
		t.Fatalf("Ctrl+C answered %q, want dismissed", got)
	}

	h.line("hello")

	select {
	case got := <-h.s.Input():
		if got.Text != "hello" {
			t.Fatalf("assistant received %q, want hello", got.Text)
		}
	case <-time.After(watchdog):
		t.Fatal("typed line did not reach the assistant")
	}

	h.cancel()

	if err := h.stop(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}

	p.unanswered(t)
}

// TestSimpleCancelWhileHandingInputOver cancels while Run is parked handing a
// typed line to an assistant that no longer receives input.
func TestSimpleCancelWhileHandingInputOver(t *testing.T) {
	for range rounds {
		h := start(t, t.Context())

		h.line("hello")
		waitForStacks(t, "simple.(*Simple).handleReadlineResult", true)
		h.cancel()

		if err := h.stop(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	}
}
