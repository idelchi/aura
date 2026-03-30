package tui

import (
	"context"
	"os"

	"github.com/idelchi/aura/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// HintFunc resolves a slash command name (e.g. "/hello") to its hint text (e.g. "<name> [arguments]").
type HintFunc func(name string) string

// UI implements a bubbletea-based TUI.
type UI struct {
	status      ui.Status          // Status bar information
	events      chan ui.Event      // Channel for receiving assistant events
	input       chan ui.UserInput  // Channel for sending user input
	actions     chan ui.UserAction // Channel for sending actions
	cancel      chan struct{}      // Channel for cancel requests
	program     *tea.Program       // Bubbletea program instance
	historyPath string             // Path for persistent input history file
	output      *os.File           // Override bubbletea output (nil = default os.Stdout)
	hintFunc    HintFunc           // Resolves slash command hints
	workdir     string             // Working directory for directive autocomplete
}

// New creates a new TUI with the given status information and history file path.
func New(status ui.Status, historyPath string, output *os.File) *UI {
	return &UI{
		status:      status,
		events:      make(chan ui.Event, 100),
		input:       make(chan ui.UserInput),
		actions:     make(chan ui.UserAction),
		cancel:      make(chan struct{}),
		historyPath: historyPath,
		output:      output,
	}
}

// Cancel returns the channel for receiving cancel requests.
func (u *UI) Cancel() <-chan struct{} {
	return u.cancel
}

// SetHintFunc sets the function used to resolve slash command hints.
func (u *UI) SetHintFunc(fn func(string) string) {
	u.hintFunc = fn
}

// SetWorkdir sets the working directory for directive autocomplete.
func (u *UI) SetWorkdir(dir string) {
	u.workdir = dir
}

// Events returns the channel for receiving events from the assistant.
func (u *UI) Events() chan<- ui.Event {
	return u.events
}

// Input returns the channel for sending user input to the assistant.
func (u *UI) Input() <-chan ui.UserInput {
	return u.input
}

// Actions returns the channel for sending UI actions to the assistant.
func (u *UI) Actions() <-chan ui.UserAction {
	return u.actions
}

// Run starts the TUI, blocking until exit.
func (u *UI) Run(ctx context.Context) error {
	model := NewModel(u.status, u.input, u.actions, u.cancel, u.historyPath, u.hintFunc, u.workdir)

	var opts []tea.ProgramOption

	if u.output != nil {
		opts = append(opts, tea.WithOutput(u.output))
	}

	u.program = tea.NewProgram(model, opts...)

	// Bridge events to bubbletea
	go u.bridgeEvents(ctx)

	_, err := u.program.Run()

	return err
}

// bridgeEvents reads from the events channel and sends to the tea.Program.
func (u *UI) bridgeEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-u.events:
			if !ok {
				return
			}

			if u.program != nil {
				u.program.Send(event)
			}
		}
	}
}
