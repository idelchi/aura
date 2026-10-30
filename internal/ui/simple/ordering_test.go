package simple

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// terminalRecorder stands in for the terminal: a pipe whose bytes, written by Simple's text
// writer and by readline alike, are captured in exactly the order they were written.
type terminalRecorder struct {
	w    *os.File
	buf  bytes.Buffer
	done chan struct{}
}

func newTerminalRecorder(t *testing.T) *terminalRecorder {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	rec := &terminalRecorder{w: w, done: make(chan struct{})}

	go func() {
		defer close(rec.done)

		io.Copy(&rec.buf, r)
		r.Close()
	}()

	return rec
}

// close ends the recording and returns everything written to the terminal.
func (r *terminalRecorder) close() string {
	r.w.Close()
	<-r.done

	return r.buf.String()
}

// visibleLines approximates what remains on screen line by line: readline cleans a line
// with "erase line, carriage return" before redrawing, so only text after the last erase
// in a line survives. Styling is dropped.
func visibleLines(stream string) []string {
	segments := strings.Split(stream, "\n")
	lines := make([]string, 0, len(segments))

	for _, segment := range segments {
		if i := strings.LastIndex(segment, "\x1b[2K"); i >= 0 {
			segment = segment[i:]
		}

		lines = append(lines, strings.ReplaceAll(ansi.Strip(segment), "\r", ""))
	}

	return lines
}

// The --output regression: text written through a mirror pipe reached the terminal after
// readline's later prompt redraws, so prompts ended up inside tool lines and prose. Text and
// readline must reach the terminal in program order, and prompts must never reach the log.
func TestSimpleTextAndPromptReachTerminalInOrder(t *testing.T) {
	terminal := newTerminalRecorder(t)

	var log bytes.Buffer

	h := &activityHarness{
		harness: prepare(t),
		clock:   &testClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)},
		ticks:   make(chan time.Time),
	}

	h.s.out = io.MultiWriter(terminal.w, &log)
	h.s.terminal = terminal.w
	h.s.interactive = true
	h.s.clock = h.clock.Now
	h.s.ticks = h.ticks
	h.run(t.Context())

	// Readline has drawn its first prompt and waits for keys.
	waitForStacks(t, "readline.(*Operation).Runes", true)

	running := &call.Call{ID: "1", Name: "Bash"}
	running.SetArgs(map[string]any{"command": "sleep 3"})
	running.MarkRunning()

	completed := *running
	completed.Complete("slept", 1)

	pending := *running
	pending.State = call.Pending

	h.events(
		ui.UserMessagesProcessed{Texts: []string{"check"}},
		ui.MessageAdded{Message: message.Message{Role: roles.User, Content: "check"}},
		ui.MessageStarted{MessageID: "msg-1"},
		ui.MessagePartAdded{MessageID: "msg-1", Part: part.Part{Type: part.Content, Text: "Let me check."}},
		ui.MessagePartAdded{MessageID: "msg-1", Part: part.Part{Type: part.Tool, Call: &pending}},
		ui.MessagePartUpdated{MessageID: "msg-1", PartIndex: 1, Part: part.Part{Type: part.Tool, Call: running}},
	)

	h.clock.advance(4 * time.Second)
	h.tick()

	h.events(
		ui.MessagePartUpdated{MessageID: "msg-1", PartIndex: 1, Part: part.Part{Type: part.Tool, Call: &completed}},
		ui.MessagePartAdded{MessageID: "msg-1", Part: part.Part{Type: part.Content, Text: "Done sleeping."}},
		ui.MessageFinalized{Message: message.Message{ID: "msg-1", Role: roles.Assistant}},
		ui.AssistantDone{},
	)

	h.cancel()
	h.stop()

	stream := terminal.close()
	lines := visibleLines(stream)

	// A prompt is only ever alone on its line: never before or after text. Readline may
	// follow the prompt with cursor positioning (" \b").
	for _, line := range lines {
		i := strings.Index(line, "] > ")
		if i < 0 {
			continue
		}

		if !strings.HasPrefix(line, "[") || strings.Trim(line[i+len("] > "):], " \b") != "" {
			t.Errorf("activity prompt shares a line with text: %q", line)
		}
	}

	// Every piece of text is on screen, unbroken, in order.
	at := 0

	for _, want := range []string{"Let me check.", "[Tool: Bash]", `"command": "sleep 3"`, "→ slept", "Done sleeping."} {
		found := false

		for i := at; i < len(lines); i++ {
			if strings.HasSuffix(lines[i], want) && !strings.Contains(lines[i], "] > ") {
				at, found = i+1, true

				break
			}
		}

		if !found {
			t.Errorf("text %q missing from the terminal after line %d:\n%s", want, at, strings.Join(lines, "\n"))
		}
	}

	if !strings.Contains(stream, "[Running Bash · 4s · last update 4s ago] > ") {
		t.Errorf("activity prompt never drawn on the terminal:\n%s", strings.Join(lines, "\n"))
	}

	// The log gets the text and none of the progress.
	plain := ansi.Strip(log.String())

	for _, want := range []string{"Let me check.", "[Tool: Bash]", "→ slept", "Done sleeping."} {
		if !strings.Contains(plain, want) {
			t.Errorf("log misses %q:\n%s", want, plain)
		}
	}

	for _, progress := range []string{"] > ", "Running Bash ·", "Waiting for model", "Preparing tools"} {
		if strings.Contains(plain, progress) {
			t.Errorf("log contains progress %q:\n%s", progress, plain)
		}
	}
}
