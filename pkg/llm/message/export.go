package message

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// ExportMessage is a clean representation of a message for export.
// It excludes internal fields (tokens, images, thinking signature, etc.).
type ExportMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	Thinking   string      `json:"thinking,omitempty"`
	ToolCalls  []call.Call `json:"tool_calls,omitempty"`
	ToolName   string      `json:"tool_name,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	CreatedAt  *time.Time  `json:"created_at,omitempty"`
}

// ExportConversation wraps exported messages for JSON output.
type ExportConversation struct {
	Messages []ExportMessage `json:"messages"`
}

// exportMessage converts a Message to an ExportMessage.
func exportMessage(msg Message) ExportMessage {
	em := ExportMessage{
		Role:       string(msg.Role),
		Content:    msg.Content,
		Thinking:   msg.Thinking,
		ToolCalls:  msg.Calls,
		ToolName:   msg.ToolName,
		ToolCallID: msg.ToolCallID,
	}

	if !msg.CreatedAt.IsZero() {
		t := msg.CreatedAt

		em.CreatedAt = &t
	}

	return em
}

// ExportMessages converts the message history to clean export structs for human-readable formats.
// Excludes Synthetic, Ephemeral, Metadata.
func (ms Messages) ExportMessages() []ExportMessage {
	filtered := ms.ForExport()
	out := make([]ExportMessage, 0, len(filtered))

	for _, msg := range filtered {
		out = append(out, exportMessage(msg))
	}

	return out
}

// ExportStructuredMessages converts the message history to clean export structs for machine-readable formats.
// Excludes Synthetic, Ephemeral. Includes Metadata.
func (ms Messages) ExportStructuredMessages() []ExportMessage {
	filtered := ms.ForExportStructured()
	out := make([]ExportMessage, 0, len(filtered))

	for _, msg := range filtered {
		out = append(out, exportMessage(msg))
	}

	return out
}

// ExportJSON returns the conversation as indented JSON.
// Uses structured export (includes Metadata) for machine-readable output.
func (ms Messages) ExportJSON() ([]byte, error) {
	conv := ExportConversation{Messages: ms.ExportStructuredMessages()}

	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling JSON: %w", err)
	}

	return append(data, '\n'), nil
}

// ExportJSONL returns the conversation as newline-delimited JSON (one message per line).
// Uses structured export (includes Metadata) for machine-readable output.
func (ms Messages) ExportJSONL() ([]byte, error) {
	exported := ms.ExportStructuredMessages()

	var buf strings.Builder

	for _, msg := range exported {
		data, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("marshaling JSONL: %w", err)
		}

		buf.Write(data)
		buf.WriteByte('\n')
	}

	return []byte(buf.String()), nil
}

// RenderMarkdown returns the conversation as formatted Markdown.
// Tool results are paired inline under their matching tool call.
func (ms Messages) RenderMarkdown() string {
	filtered := ms.ForExport()

	// Index tool-result messages by ToolCallID for pairing.
	toolResults := make(map[string]Message, len(filtered))
	for _, msg := range filtered {
		if msg.Role == roles.Tool && msg.ToolCallID != "" {
			toolResults[msg.ToolCallID] = msg
		}
	}

	// Track which tool-result messages were paired (to render orphans standalone).
	paired := make(map[string]bool, len(toolResults))

	var b strings.Builder

	first := true

	for _, msg := range filtered {
		// Skip tool-role messages here — they're rendered under their matching call.
		if msg.Role == roles.Tool {
			continue
		}

		// Bookmark renders as a labeled divider.
		if msg.IsBookmark() {
			b.WriteString("\n---\n\n")
			b.WriteString("*")
			b.WriteString(strings.TrimSpace(msg.Content))
			b.WriteString("*\n")

			first = false

			continue
		}

		// DisplayOnly renders as a notice block.
		if msg.IsDisplayOnly() {
			if !first {
				b.WriteString("\n---\n\n")
			}

			b.WriteString("> **[Notice]** ")
			b.WriteString(strings.TrimSpace(msg.Content))
			b.WriteByte('\n')

			first = false

			continue
		}

		if !first {
			b.WriteString("\n---\n\n")
		}

		first = false

		switch msg.Role {
		case roles.System:
			b.WriteString("## System\n\n")
			b.WriteString(strings.TrimSpace(msg.Content))
			b.WriteByte('\n')

		case roles.User:
			b.WriteString("## User\n\n")
			b.WriteString(strings.TrimSpace(msg.Content))
			b.WriteByte('\n')

		case roles.Assistant:
			b.WriteString("## Assistant\n\n")

			if t := strings.TrimSpace(msg.Thinking); t != "" {
				b.WriteString("> **Thinking:** ")
				b.WriteString(strings.ReplaceAll(t, "\n", "\n> "))
				b.WriteString("\n\n")
			}

			if c := strings.TrimSpace(msg.Content); c != "" {
				b.WriteString(c)
				b.WriteByte('\n')
			}

			for _, tc := range msg.Calls {
				b.WriteString("\n### Tool: ")
				b.WriteString(tc.Name)
				b.WriteByte('\n')

				if len(tc.Arguments) > 0 {
					argsJSON, err := json.MarshalIndent(tc.Arguments, "", "  ")
					if err == nil {
						b.WriteString("\n**Arguments:**\n```json\n")
						b.Write(argsJSON)
						b.WriteString("\n```\n")
					}
				}

				if result, ok := toolResults[tc.ID]; ok {
					paired[tc.ID] = true

					b.WriteString("\n**Result:**\n```\n")
					b.WriteString(strings.TrimSpace(result.Content))
					b.WriteString("\n```\n")
				}
			}
		}
	}

	// Render any orphaned tool results that weren't paired.
	for _, msg := range filtered {
		if msg.Role != roles.Tool || paired[msg.ToolCallID] {
			continue
		}

		b.WriteString("\n---\n\n")
		b.WriteString("## Tool: ")
		b.WriteString(msg.ToolName)
		b.WriteByte('\n')

		b.WriteString("\n```\n")
		b.WriteString(strings.TrimSpace(msg.Content))
		b.WriteString("\n```\n")
	}

	return b.String()
}
