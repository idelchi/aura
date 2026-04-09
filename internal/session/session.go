package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/uuid"

	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/usage"
)

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
	SessionApprovals map[string]bool `json:"session_approvals,omitempty"`
	Stats            *stats.Stats    `json:"stats,omitempty"`
	CumulativeUsage  *usage.Usage    `json:"cumulative_usage,omitempty"`
}

// Session is a complete conversation snapshot.
type Session struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Messages  message.Messages `json:"messages"`
	Meta      Meta             `json:"meta"`
	Todos     *todo.List       `json:"todos,omitempty"`
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
	return uuid.NewString()
}
