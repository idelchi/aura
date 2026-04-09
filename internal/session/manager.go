package session

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/pkg/llm/message"
)

// Manager orchestrates session save, resume, and fork operations.
type Manager struct {
	store       *Store
	active      *Session
	pendingName string // eager name for the next new session (set via SetName before first Save)
}

// NewManager creates a Manager backed by the given Store.
func NewManager(store *Store) *Manager {
	return &Manager{store: store}
}

// Save persists the current conversation state. If a session is already active
// (previously saved or resumed), it updates that session; otherwise creates a new one.
func (m *Manager) Save(title string, meta Meta, msgs message.Messages, todos *todo.List) (*Session, error) {
	now := time.Now()

	isNew := m.active == nil
	if isNew {
		id := m.pendingName
		if id == "" {
			id = newID()
		}

		m.active = &Session{
			ID:        id,
			CreatedAt: now,
		}

		m.pendingName = ""
	}

	m.active.Title = title
	m.active.UpdatedAt = now
	m.active.Meta = meta
	m.active.Messages = slices.Clone(msgs.ForSave())
	m.active.Todos = todos

	if err := m.store.Save(m.active); err != nil {
		return nil, err
	}

	if isNew {
		debug.Log("[session] created id=%s messages=%d", m.active.ID, len(msgs))
	} else {
		debug.Log("[session] updated id=%s messages=%d", m.active.ID, len(msgs))
	}

	return m.active, nil
}

// Resume loads a session by ID and sets it as the active session.
func (m *Manager) Resume(id string) (*Session, error) {
	session, err := m.store.Load(id)
	if err != nil {
		return nil, err
	}

	m.active = session
	debug.Log("[session] resumed id=%s messages=%d", session.ID, len(session.Messages))

	return session, nil
}

// List returns all stored session summaries along with any warnings
// about corrupt or unreadable session files.
func (m *Manager) List() (ListResult, error) {
	return m.store.List()
}

// Fork clears the active session so the next Save creates a new one.
func (m *Manager) Fork() {
	m.active = nil
	m.pendingName = ""
}

// ActiveTitle returns the title of the current active session, or empty string.
func (m *Manager) ActiveTitle() string {
	if m.active == nil {
		return ""
	}

	return m.active.Title
}

// ActiveID returns the ID of the active session, or the pending name, or empty string.
func (m *Manager) ActiveID() string {
	if m.active != nil {
		return m.active.ID
	}

	return m.pendingName
}

// SetName assigns a custom name (ID) to the current or next session.
// For unsaved sessions, the name is stored eagerly and used by the next Save.
// For saved sessions, the file is renamed on disk.
func (m *Manager) SetName(name string) error {
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid session name %q: must not contain path separators", name)
	}

	// Allow re-setting the same name on the active session.
	if m.store.Exists(name) {
		if m.active == nil || m.active.ID != name {
			return fmt.Errorf("session %q already exists", name)
		}
	}

	if m.active == nil {
		m.pendingName = name

		return nil
	}

	if m.active.ID == name {
		return nil
	}

	oldID := m.active.ID

	if err := m.store.Rename(oldID, name); err != nil {
		return fmt.Errorf("renaming session: %w", err)
	}

	m.active.ID = name
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
