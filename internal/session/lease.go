package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// Lease is an exclusive, process-scoped claim on one session ID.
//
// Only the holder writes the session (and its automation journal). The claim is an
// advisory flock on <store>/.locks/<id>.lock, released explicitly or when the process exits,
// so a crashed holder never blocks its successor.
type Lease struct {
	id   string
	lock *flock.Flock
}

// Acquire claims the session ID without blocking. Another holder yields ErrLocked.
func (s *Store) Acquire(id string) (*Lease, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}

	dir := filepath.Join(s.dir.Path(), ".locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	lock := flock.New(filepath.Join(dir, id+".lock"))

	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("locking session %q: %w", id, err)
	}

	if !locked {
		return nil, fmt.Errorf("%w: %q", ErrLocked, id)
	}

	return &Lease{id: id, lock: lock}, nil
}

// ID returns the claimed session ID.
func (l *Lease) ID() string {
	return l.id
}

// Release gives up the claim. Releasing a nil lease is a no-op.
func (l *Lease) Release() error {
	if l == nil {
		return nil
	}

	return l.lock.Unlock()
}
