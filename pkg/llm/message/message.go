package message

import (
	"encoding/json"
	"time"

	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// Message represents a single chat message with optional tool calls.
// This unified type serves both API communication and UI display.
type Message struct {
	// ID uniquely identifies this message (for UI tracking).
	ID string
	// Role is the message sender (user, assistant, system, tool).
	Role roles.Role
	// Content is the main message text.
	Content string
	// Thinking contains reasoning or internal thought process.
	Thinking string `json:",omitempty"`
	// ThinkingSignature is an opaque signature for thinking blocks (Anthropic, Google).
	// Must be preserved unmodified when round-tripping multi-turn conversations.
	ThinkingSignature string `json:"thinking_signature,omitempty"`
	// Images contains attached images for vision models.
	Images Images `json:"images,omitempty"`
	// Calls contains tool invocation requests from the assistant.
	Calls []call.Call `json:"calls,omitempty"`
	// ToolName is the name of the tool that produced this result.
	ToolName string `json:"tool_name,omitempty"`
	// ToolCallID links this tool result to its original call.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Type classifies the message (Normal, Synthetic, Ephemeral, DisplayOnly, Bookmark, Metadata).
	Type Type `json:"type,omitempty"`
	// Parts contains ordered content for incremental UI rendering.
	Parts []part.Part `json:"-"`
	// Error is set if an error occurred during message generation.
	Error error `json:"-"`
	// Tokens holds per-message token counts (exact from API or estimated).
	Tokens Tokens `json:"tokens,omitzero"`
	// CreatedAt is when the message was created.
	CreatedAt time.Time `json:"created_at,omitzero"`
}

// IsSynthetic returns true if this is a synthetic message.
func (m Message) IsSynthetic() bool {
	return m.Type == Synthetic
}

// IsEphemeral returns true if this is an ephemeral message (visible for one turn, then pruned).
func (m Message) IsEphemeral() bool {
	return m.Type == Ephemeral
}

// IsUser returns true if the message has the user role.
func (m Message) IsUser() bool {
	return m.Role == roles.User
}

// IsAssistant returns true if the message has the assistant role.
func (m Message) IsAssistant() bool {
	return m.Role == roles.Assistant
}

// IsTool returns true if the message has the tool role.
func (m Message) IsTool() bool {
	return m.Role == roles.Tool
}

// IsSystem returns true if the message has the system role.
func (m Message) IsSystem() bool {
	return m.Role == roles.System
}

// IsDisplayOnly returns true if this is a display-only message (UI-visible, never sent to LLM).
func (m Message) IsDisplayOnly() bool {
	return m.Type == DisplayOnly
}

// IsBookmark returns true if this is a bookmark message (structural divider).
func (m Message) IsBookmark() bool {
	return m.Type == Bookmark
}

// IsMetadata returns true if this is a metadata message (structured data, not rendered).
func (m Message) IsMetadata() bool {
	return m.Type == Metadata
}

// IsInternal returns true for types that should never reach any LLM
// (neither primary chat nor compaction agent).
func (m Message) IsInternal() bool {
	return m.Type == DisplayOnly || m.Type == Bookmark || m.Type == Metadata
}

// IsEmpty returns true if the message has no content, thinking, or tool calls.
func (m Message) IsEmpty() bool {
	return m.Content == "" && m.Thinking == "" && len(m.Calls) == 0
}

// UnmarshalJSON deserializes a Message from JSON and reconstructs Parts.
// Parts has json:"-" so it's lost during serialization; this ensures Parts
// are always populated after loading from any JSON source.
func (m *Message) UnmarshalJSON(data []byte) error {
	type raw Message // avoid infinite recursion; preserves json tags including json:"-"

	if err := json.Unmarshal(data, (*raw)(m)); err != nil {
		return err
	}

	m.ReconstructParts()

	return nil
}

// ReconstructParts rebuilds the Parts slice from serialized fields.
// Called automatically by UnmarshalJSON. Can also be called directly
// when Parts need rebuilding outside of JSON deserialization.
func (m *Message) ReconstructParts() {
	switch m.Role {
	case roles.User:
		m.Parts = []part.Part{{Type: part.Content, Text: m.Content}}
	case roles.Assistant:
		m.Parts = nil

		if m.Thinking != "" {
			m.Parts = append(m.Parts, part.Part{Type: part.Thinking, Text: m.Thinking})
		}

		if m.Content != "" {
			m.Parts = append(m.Parts, part.Part{Type: part.Content, Text: m.Content})
		}

		for i := range m.Calls {
			// Normalize non-terminal states to Pending on reconstruction.
			// State has json:"-" so after deserialization it's "" (zero value).
			// This also handles Running (interrupted execution) defensively.
			switch m.Calls[i].State {
			case call.Complete, call.Error:
				// Already terminal — keep as-is.
			default:
				m.Calls[i].State = call.Pending
			}

			m.Parts = append(m.Parts, part.Part{Type: part.Tool, Call: &m.Calls[i]})
		}
	case roles.System:
		m.Parts = []part.Part{{Type: part.Content, Text: m.Content}}
	case roles.Tool:
		m.Parts = []part.Part{{Type: part.Content, Text: m.Content}}
	}
}

// Type represents the type of message.
type Type int

const (
	// Normal represents a standard message — persists, sent to LLM, saved, exported.
	Normal Type = iota
	// Synthetic represents an injector-generated message — sent to LLM one turn, ejected.
	Synthetic
	// Ephemeral represents error feedback — sent to LLM one turn, ejected with paired call.
	Ephemeral
	// DisplayOnly represents a UI notification — visible, persisted, never sent to LLM.
	DisplayOnly
	// Bookmark represents a structural divider — rendered as separator, never sent to LLM.
	Bookmark
	// Metadata represents structured data — not rendered, not sent to LLM, exported in JSON.
	Metadata
)
