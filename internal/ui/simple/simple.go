package simple

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"

	"github.com/idelchi/aura/internal/ui"
)

// activityRefresh is how often the activity prompt is redrawn to update its times.
const activityRefresh = time.Second

// readlineResult carries the output of a single Readline() call.
type readlineResult struct {
	line string
	err  error
}

// Simple implements a readline-based TUI.
//
// Run's goroutine is the sole owner of the UI state and the only receiver of the
// events channel; the readline goroutine only reports lines to it.
//
// Activity is shown in the readline prompt rather than animated over the output, and only
// while the output is at the start of a line: readline redraws its whole line, so drawing
// the prompt next to streamed text would erase it.
//
// Text and readline's prompt reach the terminal through two writers that must stay in
// program order: out writes the terminal synchronously (and the log, when mirrored),
// readline writes terminal directly.
type Simple struct {
	status      ui.Status
	hints       ui.DisplayHints
	historyPath string
	events      chan ui.Event
	input       chan ui.UserInput
	actions     chan ui.UserAction
	cancel      chan struct{}
	rl          *readline.Instance
	activity    ui.Activity
	out         io.Writer // all text: the terminal, plus the log when output is mirrored
	terminal    *os.File  // readline prompt and redraws, $EDITOR: the terminal only

	// stdin replaces readline.Stdin as the source of input when set (tests only).
	stdin io.Reader
	// interactive makes readline edit lines and draw prompts as on a terminal without
	// switching any terminal to raw mode (tests only, with stdin set).
	interactive bool
	// clock is the time source for activity; ticks, when set, replaces the refresh ticker (tests only).
	clock func() time.Time
	ticks <-chan time.Time

	// Terminal state
	lineOpen bool   // the output ends mid-line (streamed text); no prompt is drawn
	drawn    string // prompt last drawn by redraw; empty when none is on screen

	// Streaming state
	tracker        ui.PartTracker
	lastErrorShown bool // Prevents double-display between MessageFinalized and AssistantDone

	// Ask tool state — accessed only on Run's goroutine
	pendingAsk *ui.AskRequired

	// Tool confirmation state — accessed only on Run's goroutine
	pendingConfirm *ui.ToolConfirmRequired
}

// New creates a new Simple UI with the given status and optional history file path.
// All text is written to out, synchronously. Readline draws its prompt and $EDITOR runs on
// terminal, the terminal behind out: progress redraws never reach a log that out also feeds.
func New(status ui.Status, historyPath string, out io.Writer, terminal *os.File) *Simple {
	return &Simple{
		status:      status,
		historyPath: historyPath,
		events:      make(chan ui.Event, 100),
		input:       make(chan ui.UserInput),
		actions:     make(chan ui.UserAction),
		cancel:      make(chan struct{}),
		out:         out,
		terminal:    terminal,
		clock:       time.Now,
	}
}

// Events returns the channel for receiving events from the assistant.
func (s *Simple) Events() chan<- ui.Event {
	return s.events
}

// Input returns the channel for sending user input to the assistant.
func (s *Simple) Input() <-chan ui.UserInput {
	return s.input
}

// Actions returns the channel for sending UI actions to the assistant.
func (s *Simple) Actions() <-chan ui.UserAction {
	return s.actions
}

// Cancel returns the channel for receiving cancel requests.
func (s *Simple) Cancel() <-chan struct{} {
	return s.cancel
}

// Run starts the Simple UI, blocking until exit. Its goroutine renders events in
// the order they arrive and answers input. However Run ends — cancellation, end of
// input or an error — it first answers everything still waiting (see shutdown).
func (s *Simple) Run(ctx context.Context) error {
	source := s.stdin
	if source == nil {
		source = readline.Stdin
	}

	// readline's own default input, kept so Run can close it: Instance.Close waits
	// for a read already in progress but does not close the input it wraps, so
	// without this it would wait for the next keystroke.
	input := readline.NewCancelableStdin(source)

	config := &readline.Config{
		Prompt:          s.prompt(),
		HistoryFile:     s.historyPath,
		InterruptPrompt: "^C",
		UniqueEditLine:  true,
		Stdin:           input,
		Stdout:          s.terminal,
		Stderr:          s.terminal,
	}

	if s.stdin != nil {
		config.FuncIsTerminal = func() bool { return s.interactive }
	}

	if s.interactive {
		config.FuncMakeRaw = func() error { return nil }
		config.FuncExitRaw = func() error { return nil }
		config.FuncGetWidth = func() int { return 120 }
		config.FuncOnWidthChanged = func(func()) {}
	}

	rl, err := readline.NewEx(config)
	if err != nil {
		return err
	}

	s.rl = rl

	lines := make(chan readlineResult, 1)

	defer func() {
		s.shutdown()
		// End the read in progress, then wait for the readline goroutine to return,
		// discarding what it still reports, before closing readline: Close is not
		// safe against a running Readline.
		input.Close()

		for range lines {
		}

		s.rl.Close()
	}()

	// Welcome first: readline draws its prompt as soon as it starts reading.
	s.printWelcome()

	s.drawn = config.Prompt

	go s.readlineLoop(lines)

	ticks := s.ticks
	if ticks == nil {
		ticker := time.NewTicker(activityRefresh)
		defer ticker.Stop()

		ticks = ticker.C
	}

	events := (<-chan ui.Event)(s.events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticks:
			s.tick()

		case event, ok := <-events:
			if !ok {
				events = nil // No more events; input still works.

				continue
			}

			s.handleEvent(event)

		case res, ok := <-lines:
			if !ok {
				return io.EOF
			}

			if exit, err := s.handleReadlineResult(ctx, res); exit {
				return err
			}

			s.redraw()
		}
	}
}

// shutdown runs on Run's goroutine whenever Run returns. Nobody is left to answer,
// so a rendered confirmation is denied and a rendered question dismissed, queued
// events are drained the same way (ui.DrainEvents) and flush waiters released.
// Cancellation never manufactures an answer or a permission.
func (s *Simple) shutdown() {
	if s.pendingConfirm != nil {
		s.pendingConfirm.Response <- ui.ConfirmDeny
		s.pendingConfirm = nil
	}

	if s.pendingAsk != nil {
		s.pendingAsk.Response <- ""
		s.pendingAsk = nil
	}

	ui.DrainEvents(s.events)
}

// readlineLoop runs Readline() in a dedicated goroutine, reporting each result to
// Run, which receives until lines is closed, so a send never blocks for good.
func (s *Simple) readlineLoop(lines chan<- readlineResult) {
	defer close(lines)

	for {
		line, err := s.rl.Readline()
		lines <- readlineResult{line, err}

		if err != nil && !errors.Is(err, readline.ErrInterrupt) {
			return // EOF or fatal (including Run closing input) — stop reading
		}
	}
}

// handleReadlineResult processes a readline result on Run's goroutine.
// Returns (exit bool, err error) — when exit is true, Run() should return err.
func (s *Simple) handleReadlineResult(ctx context.Context, res readlineResult) (bool, error) {
	// Readline ended its line; it draws the next prompt on a fresh one.
	s.lineOpen = false

	if res.err != nil {
		if errors.Is(res.err, readline.ErrInterrupt) {
			return s.handleInterrupt(res.line)
		}

		return true, res.err
	}

	text := strings.TrimSpace(res.line)
	if text == "" {
		return false, nil
	}

	// Check for pending confirm response
	if s.pendingConfirm != nil {
		pendingConf := s.pendingConfirm

		s.pendingConfirm = nil

		switch strings.TrimSpace(text) {
		case "1", "y", "yes", "allow":
			pendingConf.Response <- ui.ConfirmAllow
		case "2":
			pendingConf.Response <- ui.ConfirmAllowSession
		case "3":
			pendingConf.Response <- ui.ConfirmAllowPatternProject
		case "4":
			pendingConf.Response <- ui.ConfirmAllowPatternGlobal
		default:
			pendingConf.Response <- ui.ConfirmDeny
		}

		return false, nil
	}

	// Check for pending ask response
	if s.pendingAsk != nil {
		pending := s.pendingAsk

		s.pendingAsk = nil

		pending.Response <- ui.ResolveAskResponse(text, pending.Options, pending.MultiSelect)

		return false, nil
	}

	// Send input to assistant; after cancellation nobody receives it.
	select {
	case s.input <- ui.UserInput{Text: text}:
	case <-ctx.Done():
	}

	return false, nil
}

// handleInterrupt processes Ctrl+C on the main goroutine.
// Returns (exit bool, err error).
func (s *Simple) handleInterrupt(line string) (bool, error) {
	// Dismiss pending confirm on Ctrl+C
	if s.pendingConfirm != nil {
		pendingConfirm := s.pendingConfirm

		s.pendingConfirm = nil
		pendingConfirm.Response <- ui.ConfirmDeny

		return false, nil
	}

	// Dismiss pending ask on Ctrl+C
	if s.pendingAsk != nil {
		pending := s.pendingAsk

		s.pendingAsk = nil

		pending.Response <- ""

		return false, nil
	}

	if s.activity.InTurn() {
		// Cancel the turn
		select {
		case s.cancel <- struct{}{}:
			s.activity.RequestCancel()
		default:
		}

		return false, nil
	}

	if len(strings.TrimSpace(line)) == 0 {
		// Exit on Ctrl+C with empty line
		successStyle.Fprintln(s.out, "\nGoodbye!")

		return true, nil
	}

	// Clear input and continue
	return false, nil
}

// prompt returns the readline prompt for the current state. It is empty while a streamed
// line is open, since nothing may be drawn next to the text.
func (s *Simple) prompt() string {
	switch {
	case s.pendingConfirm != nil:
		return successStyle.Sprint("confirm> ")
	case s.pendingAsk != nil:
		return successStyle.Sprint("answer> ")
	case s.lineOpen:
		return ""
	case s.activity.State() == ui.ActivityWorking:
		return activityStyle.Sprintf("[%s]", s.activity.Display(s.clock())) + successStyle.Sprint(" > ")
	default:
		return successStyle.Sprint(s.status.Prompt())
	}
}

// awaitingUser reports which dialog, if any, blocks the assistant on the user.
func (s *Simple) awaitingUser() ui.Awaiting {
	switch {
	case s.pendingConfirm != nil:
		return ui.AwaitingConfirmation
	case s.pendingAsk != nil:
		return ui.AwaitingAnswer
	default:
		return ui.AwaitingNothing
	}
}

// redraw brings the activity in line with the dialog state and draws the prompt when
// it changed. Nothing is drawn while a streamed line is open.
func (s *Simple) redraw() {
	s.activity.AwaitUser(s.awaitingUser(), s.clock())

	prompt := s.prompt()
	if prompt == s.drawn {
		return
	}

	s.rl.SetPrompt(prompt)

	if !s.lineOpen {
		s.rl.Refresh()
	}

	s.drawn = prompt
}

// tick refreshes the activity prompt's times. When a streamed line has been quiet for a
// while mid-turn (typically while the model generates tool arguments after its text),
// the line is ended on screen so the activity becomes visible; later text continues below.
func (s *Simple) tick() {
	if s.lineOpen && s.activity.State() == ui.ActivityWorking && s.activity.Quiet(s.clock()) >= ui.ActivityQuiet {
		fmt.Fprintln(s.out)

		s.lineOpen = false
	}

	s.redraw()
}

// clearPrompt removes the drawn prompt before printing. An open streamed line has no
// prompt and is left alone: cleaning it would erase the text.
func (s *Simple) clearPrompt() {
	if !s.lineOpen && s.drawn != "" {
		s.rl.Clean()
	}

	s.drawn = ""
}

// endLine ends an open streamed line, for output that must start on a line of its own.
func (s *Simple) endLine() {
	if s.lineOpen {
		fmt.Fprintln(s.out)

		s.lineOpen = false
	}
}

// printed records where stdout's cursor is after printing.
func (s *Simple) printed(line ui.Line) {
	switch line {
	case ui.LineOpen:
		s.lineOpen = true
	case ui.LineStart:
		s.lineOpen = false
	case ui.LineUnchanged:
	}
}

func (s *Simple) printWelcome() {
	successStyle.Fprintln(s.out, "Aura")

	for line := range strings.SplitSeq(s.status.WelcomeInfo(), "\n") {
		ui.ToolStyle.Fprintln(s.out, line)
	}

	fmt.Fprintln(s.out)
}

// SetHintFunc is a no-op for the simple UI (hints are TUI-only).
func (s *Simple) SetHintFunc(_ func(string) string) {}

// SetWorkdir is a no-op for the simple UI (autocomplete is TUI-only).
func (s *Simple) SetWorkdir(_ string) {}
