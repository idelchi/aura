package web

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// sseClient represents a single connected browser tab.
type sseClient struct {
	ch chan string
}

// sseBroker manages SSE client connections and broadcasts events.
type sseBroker struct {
	clients sync.Map
	seq     atomic.Int64
}

// AddClient registers a new SSE client and returns its ID and event channel.
func (b *sseBroker) AddClient() (string, <-chan string) {
	id := fmt.Sprintf("sse-%d", b.seq.Add(1))
	ch := make(chan string, 100)
	b.clients.Store(id, &sseClient{ch: ch})

	return id, ch
}

// RemoveClient deregisters an SSE client.
func (b *sseBroker) RemoveClient(id string) {
	b.clients.Delete(id)
}

// Broadcast sends a named SSE event to all connected clients.
// Multi-line data is handled per SSE spec (each line prefixed with "data: ").
func (b *sseBroker) Broadcast(event, data string) {
	msg := formatSSE(event, data)

	b.clients.Range(func(_, value any) bool {
		c := value.(*sseClient)
		select {
		case c.ch <- msg:
		default:
			// Drop if buffer full — client is too slow
		}

		return true
	})
}

// formatSSE formats a named SSE event with multi-line data support.
func formatSSE(event, data string) string {
	var msg strings.Builder

	msg.WriteString("event: ")
	msg.WriteString(event)
	msg.WriteByte('\n')

	for line := range strings.SplitSeq(data, "\n") {
		msg.WriteString("data: ")
		msg.WriteString(line)
		msg.WriteByte('\n')
	}

	msg.WriteByte('\n') // Blank line terminates the event

	return msg.String()
}
