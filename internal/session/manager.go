package session

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/pkg/llm/message"
)

// Saved describes the outcome of persisting the active session.
type Saved struct {
	Session  *Session
	Revision string // sha256 of the stored bytes
	Changed  bool   // false when the content already matched the stored revision
}

// Manager orchestrates save, resume, and fork of the active session.
//
// The active session (or the reserved name of the next one) is held under a Lease,
// so no other process writes it concurrently. All methods are safe for concurrent use.
type Manager struct {
	mu          sync.Mutex
	store       *Store
	active      *Session
	lease       *Lease
	pendingName string // eager name for the next new session (set via SetName or Begin before first Save)
	revision    string // revision of the active session as last stored
	digest      string // content digest of the active session as last stored
}

// NewManager creates a Manager backed by the given Store.
func NewManager(store *Store) *Manager {
	return &Manager{store: store}
}

// Store returns the backing store.
func (m *Manager) Store() *Store {
	return m.store
}

// claim makes the manager hold the lease for id, releasing any other lease.
func (m *Manager) claim(id string) error {
	if m.lease != nil && m.lease.ID() == id {
		return nil
	}

	lease, err := m.store.Acquire(id)
	if err != nil {
		return err
	}

	m.release()
	m.lease = lease

	return nil
}

// release gives up the held lease.
func (m *Manager) release() {
	if err := m.lease.Release(); err != nil {
		debug.Log("[session] releasing lease: %v", err)
	}

	m.lease = nil
}

// Begin reserves the ID of the next new session (a UUID unless a name is pending) and
// returns it. With an active session it returns the active ID.
func (m *Manager) Begin() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active != nil {
		return m.active.ID, nil
	}

	if m.pendingName == "" {
		m.pendingName = newID()
	}

	if err := m.claim(m.pendingName); err != nil {
		return "", err
	}

	return m.pendingName, nil
}

// Save persists the current conversation state. If a session is already active
// (previously saved or resumed), it updates that session; otherwise creates a new one.
// Content identical to the stored revision is not rewritten.
func (m *Manager) Save(title string, meta Meta, msgs message.Messages, todos *todo.List) (Saved, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	candidate := Session{CreatedAt: now}

	isNew := m.active == nil
	if isNew {
		id := m.pendingName
		if id == "" {
			id = newID()
		}

		if err := m.claim(id); err != nil {
			return Saved{}, err
		}

		if m.store.Exists(id) {
			return Saved{}, fmt.Errorf("%w: %q", ErrExists, id)
		}

		candidate.ID = id
	} else {
		if m.active.Frozen {
			return Saved{}, fmt.Errorf("%w: %q", ErrFrozen, m.active.ID)
		}

		candidate = *m.active
	}

	candidate.Title = title
	candidate.Meta = meta
	candidate.Messages = slices.Clone(msgs.ForSave())
	candidate.Todos = todos

	digest, err := candidate.ContentDigest()
	if err != nil {
		return Saved{}, err
	}

	if !isNew && digest == m.digest && m.revision != "" {
		m.active = &candidate

		return Saved{Session: m.active, Revision: m.revision, Changed: false}, nil
	}

	candidate.UpdatedAt = now

	rev, err := m.store.Save(&candidate)
	if err != nil {
		return Saved{}, err
	}

	if isNew {
		m.pendingName = ""
		debug.Log("[session] created id=%s messages=%d", candidate.ID, len(msgs))
	} else {
		debug.Log("[session] updated id=%s messages=%d", candidate.ID, len(msgs))
	}

	m.active = &candidate
	m.revision = rev
	m.digest = digest

	return Saved{Session: m.active, Revision: rev, Changed: true}, nil
}

// Resume loads a session by exact ID, claims it and sets it as the active session.
// Frozen sessions are refused: fork them instead.
func (m *Manager) Resume(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	previous := m.lease

	lease := previous
	if previous == nil || previous.ID() != id {
		var err error

		lease, err = m.store.Acquire(id)
		if err != nil {
			return nil, err
		}
	}

	session, rev, err := m.store.LoadRevision(id)
	if err == nil && session.Frozen {
		err = fmt.Errorf("%w: %q; fork it to continue its conversation", ErrFrozen, id)
	}

	var digest string
	if err == nil {
		digest, err = session.ContentDigest()
	}

	if err != nil {
		if lease != previous {
			lease.Release()
		}

		return nil, err
	}

	if lease != previous {
		m.release()
		m.lease = lease
	}

	m.active = session
	m.pendingName = ""
	m.revision = rev
	m.digest = digest

	debug.Log("[session] resumed id=%s messages=%d", session.ID, len(session.Messages))

	return session, nil
}

// List returns all stored session summaries along with any warnings
// about corrupt or unreadable session files.
func (m *Manager) List() (ListResult, error) {
	return m.store.List()
}

// Fork detaches from the active session so the next Save creates a new one.
func (m *Manager) Fork() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.release()

	m.active = nil
	m.pendingName = ""
	m.revision = ""
	m.digest = ""
}

// Close releases the held session lease.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.release()
}

// ActiveTitle returns the title of the current active session, or empty string.
func (m *Manager) ActiveTitle() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return ""
	}

	return m.active.Title
}

// ActiveID returns the ID of the active session, or the pending name, or empty string.
func (m *Manager) ActiveID() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active != nil {
		return m.active.ID
	}

	return m.pendingName
}

// ActiveRevision returns the stored revision of the active session, or empty string.
func (m *Manager) ActiveRevision() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.revision
}

// ActiveInfo describes the active session as last stored. It reports false before the first save.
func (m *Manager) ActiveInfo() (Info, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil || m.revision == "" {
		return Info{}, false
	}

	return m.active.Info(m.revision), true
}

// SetName assigns a custom name (ID) to the current or next session.
// For unsaved sessions, the name is reserved and used by the next Save.
// For saved sessions, the stored session is moved to the new ID.
func (m *Manager) SetName(name string) error {
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid session name %q: must not contain path separators", name)
	}

	if err := ValidateID(name); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active != nil && m.active.ID == name {
		return nil
	}

	if m.store.Exists(name) {
		return fmt.Errorf("session %q already exists", name)
	}

	if m.active == nil {
		if err := m.claim(name); err != nil {
			return err
		}

		m.pendingName = name

		return nil
	}

	lease, err := m.store.Acquire(name)
	if err != nil {
		return err
	}

	oldID := m.active.ID

	rev, err := m.store.Rename(oldID, name)
	if err != nil {
		lease.Release()

		return fmt.Errorf("renaming session: %w", err)
	}

	m.release()
	m.lease = lease
	m.active.ID = name
	m.revision = rev
	debug.Log("[session] renamed %s → %s", oldID, name)

	return nil
}

// Find resolves a session ID prefix to a full session ID.
func (m *Manager) Find(prefix string) (string, error) {
	return m.store.Find(prefix)
}

// ListDisplay returns a formatted string of all sessions for display.
func (m *Manager) ListDisplay() (string, error) {
	result, err := m.List()
	if err != nil {
		return "", err
	}

	if len(result.Summaries) == 0 {
		return "No saved sessions", nil
	}

	var b strings.Builder

	for _, w := range result.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}

	for _, s := range result.Summaries {
		fmt.Fprintln(&b, s.Display())
	}

	return strings.TrimRight(b.String(), "\n"), nil
}
