package ui

// DrainEvents processes remaining events after context cancellation.
// It unblocks any pending response channels (Flush.Done, ToolConfirmRequired.Response,
// AskRequired.Response) to prevent goroutine deadlocks, then returns when the
// channel is empty or closed. Nobody is left to answer, so a confirmation is denied
// and a question is dismissed — cancellation never manufactures permission or an answer.
func DrainEvents(events <-chan Event) {
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}

			switch e := event.(type) {
			case Flush:
				close(e.Done)
			case ToolConfirmRequired:
				e.Response <- ConfirmDeny
			case AskRequired:
				e.Response <- ""
			}
		default:
			return
		}
	}
}
