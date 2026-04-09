// Package part defines message part types for content, thinking, and tool calls.
package part

import "github.com/idelchi/aura/pkg/llm/tool/call"

// Type identifies the kind of content in a message part.
type Type string

const (
	Thinking Type = "thinking"
	Content  Type = "content"
	Tool     Type = "tool"
)

// Part represents a single part of a message for incremental UI rendering.
type Part struct {
	// Type identifies what kind of part this is.
	Type Type
	// Text holds the content for thinking/content parts.
	Text string
	// Call holds the tool call information for tool parts.
	Call *call.Call
}

// IsContent returns true if this is a content part.
func (p Part) IsContent() bool {
	return p.Type == Content
}

// IsThinking returns true if this is a thinking part.
func (p Part) IsThinking() bool {
	return p.Type == Thinking
}

// IsTool returns true if this is a tool call part.
func (p Part) IsTool() bool {
	return p.Type == Tool
}
