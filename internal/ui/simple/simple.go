package simple

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/chzyer/readline"

	"github.com/idelchi/aura/internal/spintext"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/spinner"
)

// State represents the UI state.
type State int

const (
	StateIdle State = iota
	StateStreaming
)

// readlineResult carries the output of a single Readline() call.
type readlineResult struct {
	line string
	err  error
}

// Simple implements a readline-based TUI.
//
// Run's goroutine is the sole owner of the UI state and the only receiver of the
// events channel; the readline goroutine only reports lines to it.
type Simple struct {
	status      ui.Status
	hints       ui.DisplayHints
	historyPath string
	events      chan ui.Event
	input       chan ui.UserInput
	actions     chan ui.UserAction
	cancel      chan struct{}
	rl          *readline.Instance
	spinner     *spinner.Spinner
	spintext    *spintext.SpinText
	state       State

	// stdin replaces readline.Stdin as the source of input when set (tests only).
	stdin io.Reader

	// Streaming state
	tracker        ui.PartTracker
	lastErrorShown bool // Prevents double-display between MessageFinalized and AssistantDone

	// Ask tool state — accessed only on Run's goroutine
	pendingAsk *ui.AskRequired

	// Tool confirmation state — accessed only on Run's goroutine
	pendingConfirm *ui.ToolConfirmRequired
}

// New creates a new Simple UI with the given status and optional history file path.
func New(status ui.Status, historyPath string) *Simple {
	st := spintext.Default()

	return &Simple{
		status:      status,
		historyPath: historyPath,
		events:      make(chan ui.Event, 100),
		input:       make(chan ui.UserInput),
		actions:     make(chan ui.UserAction),
		cancel:      make(chan struct{}),
		spintext:    st,
		spinner:     spinner.New(st.Random()),
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
	}

	if s.stdin != nil {
		config.FuncIsTerminal = func() bool { return false }
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

	go s.readlineLoop(lines)

	s.printWelcome()

	events := (<-chan ui.Event)(s.events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

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
		}
	}
}

// shutdown runs on Run's goroutine whenever Run returns. Nobody is left to answer,
// so a rendered confirmation is denied and a rendered question dismissed, queued
// events are drained the same way (ui.DrainEvents) and flush waiters released.
// Cancellation never manufactures an answer or a permission.
func (s *Simple) shutdown() {
	s.spinner.Stop()

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
	if res.err != nil {
		if errors.Is(res.err, readline.ErrInterrupt) {
			return s.handleInterrupt(res.line)
		}

		return true, res.err
	}

	text := strings.TrimSpace(res.line)
	if text == "" {
		s.rl.SetPrompt(s.prompt())

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

		s.rl.SetPrompt(s.prompt())

		return false, nil
	}

	// Check for pending ask response
	if s.pendingAsk != nil {
		pending := s.pendingAsk

		s.pendingAsk = nil

		pending.Response <- ui.ResolveAskResponse(text, pending.Options, pending.MultiSelect)

		s.rl.SetPrompt(s.prompt())

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

		s.rl.SetPrompt(s.prompt())

		return false, nil
	}

	// Dismiss pending ask on Ctrl+C
	if s.pendingAsk != nil {
		pending := s.pendingAsk

		s.pendingAsk = nil

		pending.Response <- ""

		s.rl.SetPrompt(s.prompt())

		return false, nil
	}

	if s.state == StateStreaming {
		// Cancel streaming
		select {
		case s.cancel <- struct{}{}:
		default:
		}

		return false, nil
	}

	if len(strings.TrimSpace(line)) == 0 {
		// Exit on Ctrl+C with empty line
		successStyle.Println("\nGoodbye!")

		return true, nil
	}

	// Clear input and continue
	return false, nil
}

func (s *Simple) prompt() string {
	if s.state == StateStreaming {
		return ""
	}

	return successStyle.Sprint(s.status.Prompt())
}

func (s *Simple) printWelcome() {
	successStyle.Println("Aura")

	for line := range strings.SplitSeq(s.status.WelcomeInfo(), "\n") {
		ui.ToolStyle.Println(line)
	}

	fmt.Println()
}

// SetHintFunc is a no-op for the simple UI (hints are TUI-only).
func (s *Simple) SetHintFunc(_ func(string) string) {}

// SetWorkdir is a no-op for the simple UI (autocomplete is TUI-only).
func (s *Simple) SetWorkdir(_ string) {}
