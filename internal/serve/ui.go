package serve

import (
	"context"

	"github.com/idelchi/aura/internal/ui"
)

// UI is the engine-facing side of the server: a ui.UI whose input comes from accepted
// runs and whose events feed the journal. It is created before the assistant and
// attached to its Server before the engine loop starts.
type UI struct {
	events chan ui.Event
	input  chan ui.UserInput
	cancel chan struct{}
	server *Server
}

// NewUI creates the engine adapter.
func NewUI() *UI {
	return &UI{
		events: make(chan ui.Event, 1024),
		input:  make(chan ui.UserInput),
		cancel: make(chan struct{}, 1),
	}
}

func (u *UI) Events() chan<- ui.Event           { return u.events }
func (u *UI) Input() <-chan ui.UserInput        { return u.input }
func (u *UI) Actions() <-chan ui.UserAction     { return nil }
func (u *UI) Cancel() <-chan struct{}           { return u.cancel }
func (u *UI) SetHintFunc(_ func(string) string) {}
func (u *UI) SetWorkdir(_ string)               {}

// Run feeds engine events to the attached server until ctx ends, then releases any
// engine goroutine still waiting for an answer — denying, never approving.
func (u *UI) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			u.drain()

			return ctx.Err()
		case event := <-u.events:
			u.server.handle(event)
		}
	}
}

// drain unblocks waiting engine goroutines after shutdown: confirmations are denied and
// questions dismissed (unlike interactive UIs, automation never approves on its own).
func (u *UI) drain() {
	for {
		select {
		case event := <-u.events:
			switch e := event.(type) {
			case ui.Flush:
				close(e.Done)
			case ui.ToolConfirmRequired:
				e.Response <- ui.ConfirmDeny
			case ui.AskRequired:
				e.Response <- ""
			}
		default:
			return
		}
	}
}

// signalCancel asks the engine to cancel the running turn without blocking.
func (u *UI) signalCancel() {
	select {
	case u.cancel <- struct{}{}:
	default:
	}
}

// clearCancel drops a cancellation signal the engine has not consumed, so it cannot
// reach a later run.
func (u *UI) clearCancel() {
	select {
	case <-u.cancel:
	default:
	}
}
