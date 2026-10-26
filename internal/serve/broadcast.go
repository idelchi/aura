package serve

import (
	"sync"

	"github.com/idelchi/aura/pkg/automation"
)

// transient is one unsequenced live event (content deltas, tool output, spinner).
type transient struct {
	Type automation.EventType
	Data any
}

// broadcaster fans transient events out to live subscribers. Delivery is best effort:
// a subscriber whose buffer is full misses events, which the durable journal supersedes.
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan transient]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{subs: make(map[chan transient]struct{})}
}

// subscribe returns a channel of transient events and its cancel function.
func (b *broadcaster) subscribe() (<-chan transient, func()) {
	ch := make(chan transient, 256)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// publish offers an event to every subscriber without blocking.
func (b *broadcaster) publish(e transient) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
