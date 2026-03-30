package ui

// DrainEvents processes remaining events after context cancellation.
// It unblocks any pending response channels (Flush.Done, ToolConfirmRequired.Response,
// AskRequired.Response) to prevent goroutine deadlocks, then returns when the
// channel is empty.
func DrainEvents(events <-chan Event) {
	for {
		select {
		case event := <-events:
			switch e := event.(type) {
			case Flush:
				close(e.Done)
			case ToolConfirmRequired:
				e.Response <- ConfirmAllow
			case AskRequired:
				if len(e.Options) > 0 {
					e.Response <- e.Options[0].Label
				} else {
					e.Response <- "proceed"
				}
			}
		default:
			return
		}
	}
}
