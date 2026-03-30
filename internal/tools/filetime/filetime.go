// Package filetime tracks file read operations to enforce "read before edit" semantics.
// Each Tracker instance maintains its own read map, providing per-session isolation
// (parent assistant and each subagent get independent tracking).
package filetime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/godyl/pkg/path/file"
)

// ErrReadRequired is returned when a file must be read before editing.
var ErrReadRequired = errors.New(
	"file was modified or not yet read — read it again to see current content before making changes",
)

// normalize cleans a path for consistent map-key deduplication.
// All callers pass absolute paths (via tool.ResolvePath), so New()
// only normalizes separators and dot segments via filepath.Join.
func normalize(path string) string {
	return file.New(path).Path()
}

// Tracker tracks file reads and carries the read-before enforcement policy
// for a single session boundary (parent assistant or individual subagent).
//
// All methods are nil-safe: a nil Tracker is a no-op for mutations and
// returns sane defaults for queries. This allows tools to call
// FromContext(ctx).RecordRead(...) without nil-checking, even when no
// Tracker is on the context (e.g. aura tools subcommand).
type Tracker struct {
	reads           map[string]bool
	mu              sync.RWMutex
	policy          tool.ReadBeforePolicy
	runtimeOverride *tool.ReadBeforePolicy
}

// NewTracker creates a Tracker with the given base policy and an empty read map.
func NewTracker(policy tool.ReadBeforePolicy) *Tracker {
	return &Tracker{
		reads:  make(map[string]bool),
		policy: policy,
	}
}

// RecordRead marks a file as read.
func (t *Tracker) RecordRead(path string) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.reads[normalize(path)] = true
}

// WasRead returns true if the file was read in this tracker's session.
func (t *Tracker) WasRead(path string) bool {
	if t == nil {
		return false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.reads[normalize(path)]
}

// AssertRead returns ErrReadRequired if file was not read.
func (t *Tracker) AssertRead(path string) error {
	if t == nil {
		return nil
	}

	if !t.WasRead(path) {
		return fmt.Errorf("%w: %s", ErrReadRequired, path)
	}

	return nil
}

// ClearRead marks a file as needing to be read again.
func (t *Tracker) ClearRead(path string) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.reads, normalize(path))
}

// Policy returns the effective read-before policy.
func (t *Tracker) Policy() tool.ReadBeforePolicy {
	if t == nil {
		return tool.DefaultReadBeforePolicy()
	}

	if t.runtimeOverride != nil {
		return *t.runtimeOverride
	}

	return t.policy
}

// SetPolicy records a runtime policy override. This is called from the
// /readbefore slash command and session resume — both represent deliberate
// non-default policy that should propagate to subagents.
func (t *Tracker) SetPolicy(p tool.ReadBeforePolicy) {
	if t == nil {
		return
	}

	t.runtimeOverride = &p
}

// RuntimeOverride returns the runtime policy override, or nil if none was set.
// Used by subagent construction to decide whether to override the child's
// config-resolved policy with the parent's session-wide runtime decision.
func (t *Tracker) RuntimeOverride() *tool.ReadBeforePolicy {
	if t == nil {
		return nil
	}

	return t.runtimeOverride
}

// ── Context plumbing ─────────────────────────────────────────────────────────

type ctxKey struct{}

// WithTracker returns a derived context carrying a Tracker.
func WithTracker(ctx context.Context, t *Tracker) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// FromContext extracts the Tracker from the context, or nil.
// A nil Tracker is safe to use — all methods are nil-safe.
func FromContext(ctx context.Context) *Tracker {
	t, _ := ctx.Value(ctxKey{}).(*Tracker)

	return t
}
