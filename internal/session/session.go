package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"uuid"

	"github.com/dustin/go-humanize"

	"github.com/idelchi/aura/internal/calllimit"
	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/pkg/automation"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// Errors returned by store and manager operations. Callers classify them with errors.Is.
var (
	// ErrNotFound is returned when no session matches the given ID or prefix.
	ErrNotFound = errors.New("session not found")
	// ErrExists is returned when an operation would replace a different existing session or file.
	ErrExists = errors.New("session already exists")
	// ErrFrozen is returned when a frozen session would be saved or resumed.
	ErrFrozen = errors.New("session is frozen")
	// ErrLocked is returned when another process holds the session.
	ErrLocked = errors.New("session is in use by another process")
	// ErrRevisionMismatch is returned when a stored session differs from the expected revision.
	ErrRevisionMismatch = errors.New("session revision mismatch")
	// ErrInvalidID is returned for session IDs that cannot be used as file names.
	ErrInvalidID = errors.New("invalid session id")
	// ErrInvalidSession is returned when a file is not a valid session.
	ErrInvalidSession = errors.New("invalid session")
)

// idPattern restricts session IDs to portable file names. UUIDs and user names both match.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ValidateID reports whether id can name a session file.
func ValidateID(id string) error {
	if !idPattern.MatchString(id) || strings.Contains(id, "..") {
		return fmt.Errorf("%w %q: use letters, digits, '.', '_' or '-' (no path separators or '..')", ErrInvalidID, id)
	}

	return nil
}

// Meta captures the assistant state at the time of save.
type Meta struct {
	Agent            string                 `json:"agent"`
	Mode             string                 `json:"mode"`
	Think            string                 `json:"think"`
	Model            string                 `json:"model,omitempty"`
	Provider         string                 `json:"provider,omitempty"`
	Verbose          bool                   `json:"verbose,omitempty"`
	LoadedTools      []string               `json:"loaded_tools,omitempty"`
	ReadBeforePolicy *tool.ReadBeforePolicy `json:"read_before_policy,omitempty"`

	// Runtime toggles
	Sandbox bool `json:"sandbox,omitempty"`

	// Session-scoped state
	SessionApprovals map[string]bool            `json:"session_approvals,omitempty"`
	Stats            *stats.Stats               `json:"stats,omitempty"`
	CumulativeUsage  *usage.Usage               `json:"cumulative_usage,omitempty"`
	ToolCallUsage    map[string]calllimit.Usage `json:"tool_call_usage,omitempty"` // raw execution counts
}

// Lineage identifies the exact stored parent revision a session was forked from.
type Lineage = automation.Lineage

// Session is a complete conversation snapshot.
type Session struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Messages  message.Messages `json:"messages"`
	Meta      Meta             `json:"meta"`
	Todos     *todo.List       `json:"todos,omitempty"`
	// Frozen sessions are immutable: they can be read, exported and forked, never saved or resumed.
	Frozen bool `json:"frozen,omitempty"`
	// Parent records the stored parent revision for forks.
	Parent *Lineage `json:"parent,omitempty"`
}

// IsUUID reports whether the session ID is a UUID (as opposed to a user-chosen name).
func (s Session) IsUUID() bool {
	return isUUID(s.ID)
}

// FirstUserContent returns the content of the first real user message, or empty string.
// Excludes internal types (DisplayOnly, Bookmark, Metadata).
func (s Session) FirstUserContent() string {
	for _, msg := range s.Messages {
		if msg.Role == roles.User && !msg.IsInternal() {
			return strings.TrimSpace(msg.Content)
		}
	}

	return ""
}

// ConversationLength returns the number of stored non-system messages.
func (s Session) ConversationLength() int {
	n := 0

	for _, msg := range s.Messages {
		if msg.Role != roles.System {
			n++
		}
	}

	return n
}

// ContentDigest hashes the conversation content: title, messages, metadata and todos.
// Identity, timestamps, freezing and lineage are excluded, so equal digests mean the
// same conversation regardless of where or when it was stored.
func (s Session) ContentDigest() (string, error) {
	data, err := json.Marshal(struct {
		Title    string           `json:"title"`
		Messages message.Messages `json:"messages"`
		Meta     Meta             `json:"meta"`
		Todos    *todo.List       `json:"todos,omitempty"`
	}{s.Title, s.Messages, s.Meta, s.Todos})
	if err != nil {
		return "", fmt.Errorf("encoding session content: %w", err)
	}

	return Digest(data), nil
}

// Validate checks that a decoded session can be stored and resumed.
func (s Session) Validate() error {
	if err := ValidateID(s.ID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSession, err)
	}

	if s.Parent != nil && (s.Parent.ID == "" || s.Parent.Revision == "") {
		return fmt.Errorf("%w %q: incomplete parent lineage", ErrInvalidSession, s.ID)
	}

	return nil
}

// Info is the machine-readable description of a stored session.
type Info = automation.SessionSummary

// Info describes the session as stored with the given revision.
func (s Session) Info(revision string) Info {
	return Info{
		ID:        s.ID,
		Title:     s.Title,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Revision:  revision,
		Messages:  s.ConversationLength(),
		Frozen:    s.Frozen,
		Parent:    s.Parent,
	}
}

// Digest returns the hex sha256 of data. Session revisions are digests of stored bytes.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// Summary is a lightweight listing entry for a session.
type Summary struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	UpdatedAt        time.Time `json:"updated_at"`
	FirstUserMessage string    `json:"first_user_message"`
}

// IsUUID reports whether the session ID is a UUID (as opposed to a user-chosen name).
func (s Summary) IsUUID() bool {
	return isUUID(s.ID)
}

// isUUID reports whether id is a valid UUID string.
func isUUID(id string) bool {
	_, err := uuid.Parse(id)

	return err == nil
}

// ShortID returns a display-friendly version of the ID.
// Custom names are shown in full; UUIDs are truncated to 8 characters.
func (s Summary) ShortID() string {
	if s.IsUUID() {
		return s.ID[:8]
	}

	return s.ID
}

// Display formats the summary for human-readable output.
func (s Summary) Display() string {
	preview := s.FirstUserMessage
	if len(preview) > 60 {
		preview = preview[:60] + "..."
	}

	title := s.Title
	if title == "" {
		title = "(untitled)"
	}

	return fmt.Sprintf("%s  [%s]  %s  %s", s.ShortID(), title, humanize.Time(s.UpdatedAt), preview)
}

// PickerLabel returns the main display text for a picker item.
// Shows the title if set, otherwise falls back to a truncated first user message.
func (s Summary) PickerLabel() string {
	if s.Title != "" {
		return s.Title
	}

	if s.FirstUserMessage != "" {
		msg := s.FirstUserMessage
		if len(msg) > 60 {
			msg = msg[:60] + "..."
		}

		return msg
	}

	return "(untitled)"
}

// PickerDescription returns secondary text for a picker item.
// Shows a humanized relative time and the short session ID.
func (s Summary) PickerDescription() string {
	return fmt.Sprintf("%s · %s", humanize.Time(s.UpdatedAt), s.ShortID())
}

// ShortDisplay returns a brief session identifier.
// Custom names are shown in full; UUIDs are truncated to 8 characters.
func (s Session) ShortDisplay() string {
	id := s.ID
	if s.IsUUID() {
		id = s.ID[:8]
	}

	if s.Title != "" {
		return fmt.Sprintf("%s (%s)", id, s.Title)
	}

	return id
}

// newID generates a new UUID for a session.
func newID() string {
	return uuid.New().String()
}
