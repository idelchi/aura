// Package headless provides a UI that prints events to stdout without user input.
package headless

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// Headless prints events to stdout without readline input.
type Headless struct {
	events         chan ui.Event
	status         ui.Status
	verbose        bool
	welcomed       bool
	tracker        ui.PartTracker
	lastErrorShown bool // Prevents double-display between MessageFinalized and AssistantDone
	spinnerTimer   *time.Timer
	lastStatus     string // Last printed status header — used to suppress duplicate status bars
}

// New creates a new Headless UI.
func New() *Headless {
	return &Headless{
		events: make(chan ui.Event, 100),
	}
}

// Events returns the channel for receiving events.
func (h *Headless) Events() chan<- ui.Event {
	return h.events
}

// Input returns nil (headless has no user input).
func (h *Headless) Input() <-chan ui.UserInput { return nil }

// Actions returns nil (headless has no user actions).
func (h *Headless) Actions() <-chan ui.UserAction { return nil }

// Cancel returns nil (headless has no cancel mechanism).
func (h *Headless) Cancel() <-chan struct{} { return nil }

// SetHintFunc is a no-op for headless.
func (h *Headless) SetHintFunc(_ func(string) string) {}

// SetWorkdir is a no-op for headless.
func (h *Headless) SetWorkdir(_ string) {}

// Run consumes events and prints to stdout until context is cancelled.
func (h *Headless) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			h.stopSpinnerTimer()
			ui.DrainEvents(h.events)

			return ctx.Err()
		case event, ok := <-h.events:
			if !ok {
				return nil
			}

			h.processEvent(event)
		}
	}
}

// stopSpinnerTimer cancels any pending delayed status print.
func (h *Headless) stopSpinnerTimer() {
	if h.spinnerTimer != nil {
		h.spinnerTimer.Stop()

		h.spinnerTimer = nil
	}
}

func (h *Headless) processEvent(event ui.Event) {
	// Cancel pending spinner timer on any non-spinner event.
	// This prevents stale status lines from printing after a tool completes
	// but before the next model response arrives.
	if _, isSpinner := event.(ui.SpinnerMessage); !isSpinner {
		h.stopSpinnerTimer()
	}

	switch e := event.(type) {
	case ui.MessageAdded:
		h.handleMessageAdded(e)
	case ui.MessageStarted:
		h.tracker.Reset()
	case ui.MessagePartAdded:
		h.handleMessagePartAdded(e)
	case ui.MessagePartUpdated:
		h.handleMessagePartUpdated(e)
	case ui.MessageFinalized:
		h.tracker.Reset()

		if e.Message.Error != nil {
			ui.ErrorStyle.Printf("\nError: %v\n", e.Message.Error)

			h.lastErrorShown = true
		} else {
			fmt.Println()

			h.lastErrorShown = false
		}
	case ui.StatusChanged:
		h.status = e.Status

		// Print welcome message on first status change
		if !h.welcomed {
			fmt.Println(h.status.WelcomeInfo())
			fmt.Println()

			h.welcomed = true
		}

		if e.Status.Steps.Current == 0 {
			break
		}

		var statusParts []string

		if e.Status.Steps.Current > 0 {
			statusParts = append(statusParts, fmt.Sprintf("step %d/%d", e.Status.Steps.Current, e.Status.Steps.Max))
		}

		if td := e.Status.TokensDisplay(); td != "" {
			statusParts = append(statusParts, td)
		}

		if len(statusParts) == 0 {
			break
		}

		header := fmt.Sprintf("[%s]", strings.Join(statusParts, " • "))

		if header == h.lastStatus {
			break
		}

		h.lastStatus = header

		line := strings.Repeat("-", len(header))
		ui.ThinkingStyle.Printf("\n%s\n%s\n%s\n", line, header, line)
	case ui.DisplayHintsChanged:
		h.verbose = e.Hints.Verbose
	case ui.WaitingForInput:
		// No-op for headless
	case ui.UserMessagesProcessed:
		// No-op for headless
	case ui.SessionRestored:
		// No-op for headless
	case ui.SlashCommandHandled:
		// No-op for headless
	case ui.TodoEditRequested:
		// No-op for headless
	case ui.SpinnerMessage:
		h.stopSpinnerTimer()

		if e.Text != "" {
			text := e.Text

			h.spinnerTimer = time.AfterFunc(2*time.Second, func() {
				fmt.Fprintf(color.Error, "[status] %s\n", text)
			})
		}
	case ui.ToolOutputDelta:
		h.stopSpinnerTimer()
		fmt.Fprintf(color.Error, "  %s\n", e.Line)
	case ui.PickerOpen:
		fmt.Print(e.Display())
	case ui.SyntheticInjected:
		ui.SyntheticStyle.Printf("\n%s\n%s\n", e.Header, e.Content)
	case ui.CommandResult:
		h.handleCommandResult(e)
	case ui.AssistantDone:
		h.handleAssistantDone(e)
	case ui.ToolConfirmRequired:
		e.Response <- ui.ConfirmAllow
	case ui.AskRequired:
		if len(e.Options) > 0 {
			e.Response <- e.Options[0].Label
		} else {
			e.Response <- "proceed"
		}
	case ui.Flush:
		close(e.Done)
	}
}

func (h *Headless) handleMessageAdded(e ui.MessageAdded) {
	if e.Message.IsBookmark() {
		fmt.Printf("\n--- %s ---\n", strings.TrimSpace(e.Message.Content))

		return
	}

	if e.Message.IsDisplayOnly() {
		ui.SyntheticStyle.Printf("\n%s\n", strings.TrimSpace(e.Message.Content))

		return
	}

	if e.Message.Role == roles.User {
		ui.UserStyle.Printf("%s%s\n", h.status.UserPrompt(), strings.TrimSpace(e.Message.Content))
	}
}

func (h *Headless) handleMessagePartAdded(e ui.MessagePartAdded) {
	ui.RenderPartAdded(h.tracker.Added(e.Part), e.Part, h.verbose, h.status.AssistantPrompt())
}

func (h *Headless) handleMessagePartUpdated(e ui.MessagePartUpdated) {
	ui.RenderPartUpdated(h.tracker.Updated(e.Part), e.Part, h.verbose, h.status.AssistantPrompt())
}

func (h *Headless) handleCommandResult(e ui.CommandResult) {
	if e.Command != "" {
		ui.UserStyle.Printf("%s%s\n", h.status.UserPrompt(), e.Command)
	}

	if e.Error != nil {
		ui.ErrorStyle.Printf("Error: %v\n", e.Error)
	} else if e.Message != "" {
		fmt.Println(strings.TrimSpace(e.Message))
	}
}

func (h *Headless) handleAssistantDone(e ui.AssistantDone) {
	if e.Cancelled {
		fmt.Println("\n(cancelled)")
	} else if e.Error != nil && !h.lastErrorShown {
		ui.ErrorStyle.Printf("\nError: %v\n", e.Error)
	}

	h.lastErrorShown = false
}
