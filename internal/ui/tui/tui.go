package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/idelchi/aura/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// HintFunc resolves a slash command name (e.g. "/hello") to its hint text (e.g. "<name> [arguments]").
type HintFunc func(name string) string

// UI implements a bubbletea-based TUI.
type UI struct {
	status           ui.Status          // Status bar information
	events           chan ui.Event      // Channel for receiving assistant events
	input            chan ui.UserInput  // Channel for sending user input
	actions          chan ui.UserAction // Channel for sending actions
	cancel           chan struct{}      // Channel for cancel requests
	program          *tea.Program       // Bubbletea program instance
	historyPath      string             // Path for persistent input history file
	output           *os.File           // Override bubbletea output (nil = default os.Stdout)
	hintFunc         HintFunc           // Resolves slash command hints
	workdir          string             // Working directory for directive autocomplete
	transcript       *ui.Transcript     // Semantic output log; independent of terminal redraws
	transcriptBuffer *bufio.Writer
}

// New creates a new TUI with the given status information and history file path.
func New(status ui.Status, historyPath string, output *os.File, transcript io.Writer) *UI {
	u := &UI{
		status:      status,
		events:      make(chan ui.Event, 100),
		input:       make(chan ui.UserInput),
		actions:     make(chan ui.UserAction),
		cancel:      make(chan struct{}),
		historyPath: historyPath,
		output:      output,
	}
	if transcript != nil {
		u.transcriptBuffer = bufio.NewWriter(transcript)
		u.transcript = ui.NewTranscript(u.transcriptBuffer)
	}
	return u
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	model := NewModel(u.status, u.input, u.actions, u.cancel, u.historyPath, u.hintFunc, u.workdir)

	opts := []tea.ProgramOption{tea.WithContext(ctx)}

	if u.output != nil {
		opts = append(opts, tea.WithOutput(u.output))
	}

	u.program = tea.NewProgram(model, opts...)

	// Bridge events to bubbletea
	done := make(chan error, 1)
	go func() {
		done <- u.bridgeEvents(ctx, cancel)
	}()

	_, err := u.program.Run()
	cancel()
	if logErr := <-done; logErr != nil {
		return logErr
	}

	return err
}

// bridgeEvents reads from the events channel and sends to the tea.Program.
func (u *UI) bridgeEvents(ctx context.Context, cancel context.CancelFunc) (err error) {
	defer ui.DrainEvents(u.events)
	var tick <-chan time.Time
	if u.transcriptBuffer != nil {
		// Bound log latency without putting a filesystem write on every token.
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		tick = ticker.C
		defer func() {
			if flushErr := u.transcriptBuffer.Flush(); err == nil && flushErr != nil {
				err = fmt.Errorf("output transcript: %w", flushErr)
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
			if err := u.transcriptBuffer.Flush(); err != nil {
				cancel()
				return fmt.Errorf("output transcript: %w", err)
			}
		case event, ok := <-u.events:
			if !ok {
				return nil
			}

			if u.transcript != nil {
				if err := u.transcript.Record(event); err != nil {
					cancel()
					return fmt.Errorf("output transcript: %w", err)
				}
				switch event.(type) {
				case ui.Flush, ui.AssistantDone, ui.MessageFinalized, ui.CommandResult,
					ui.ToolConfirmRequired, ui.AskRequired:
					if err := u.transcriptBuffer.Flush(); err != nil {
						cancel()
						return fmt.Errorf("output transcript: %w", err)
					}
				}
			}

			if u.program != nil {
				u.program.Send(event)
			}
		}
	}
}
