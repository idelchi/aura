package simple

import (
	"context"
	"errors"
	"fmt"
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

	// Streaming state
	tracker        ui.PartTracker
	lastErrorShown bool // Prevents double-display between MessageFinalized and AssistantDone

	// Ask tool state — accessed only on main goroutine
	pendingAsk *ui.AskRequired

	// Tool confirmation state — accessed only on main goroutine
	pendingConfirm *ui.ToolConfirmRequired

	// Channel-serialized render commands from event goroutine
	renderCh chan func()

	// Readline results from readline goroutine
	lineCh chan readlineResult
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
		renderCh:    make(chan func(), 100),
		lineCh:      make(chan readlineResult, 1),
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

// Run starts the Simple UI, blocking until exit.
func (s *Simple) Run(ctx context.Context) error {
	var err error

	s.rl, err = readline.NewEx(&readline.Config{
		Prompt:          s.prompt(),
		HistoryFile:     s.historyPath,
		InterruptPrompt: "^C",
		UniqueEditLine:  true,
	})
	if err != nil {
		return err
	}

	defer s.rl.Close()

	// Start event consumer
	go s.consumeEvents(ctx)

	// Start readline goroutine
	go s.readlineLoop()

	// Print welcome message
	s.printWelcome()

	// Main select loop — sole owner of all mutable state
	for {
		select {
		case <-ctx.Done():
			s.spinner.Stop()
			ui.DrainEvents(s.events)

			return ctx.Err()

		case cmd := <-s.renderCh:
			cmd()

		case res := <-s.lineCh:
			if exit, err := s.handleReadlineResult(res); exit {
				return err
			}
		}
	}
}

// readlineLoop runs Readline() in a dedicated goroutine, sending results to lineCh.
func (s *Simple) readlineLoop() {
	defer close(s.lineCh)

	for {
		line, err := s.rl.Readline()
		s.lineCh <- readlineResult{line, err}

		if err != nil && !errors.Is(err, readline.ErrInterrupt) {
			return // EOF or fatal — stop reading
		}
	}
}

// handleReadlineResult processes a readline result on the main goroutine.
// Returns (exit bool, err error) — when exit is true, Run() should return err.
func (s *Simple) handleReadlineResult(res readlineResult) (bool, error) {
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

	// Send input to assistant
	s.input <- ui.UserInput{Text: text}

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

func (s *Simple) consumeEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-s.events:
			if !ok {
				return
			}

			s.processEvent(event)
		}
	}
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
