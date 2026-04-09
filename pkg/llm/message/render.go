package message

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/llm/roles"
)

// Render converts the message into a human-readable text representation.
func (m Message) Render() string {
	if m.IsBookmark() {
		return fmt.Sprintf("--- %s ---\n", strings.TrimSpace(m.Content))
	}

	if m.IsMetadata() {
		return ""
	}

	var b strings.Builder

	switch m.Role {
	case roles.Assistant:
		if t := strings.TrimSpace(m.Thinking); t != "" {
			fmt.Fprintf(&b, "[Assistant Thinking]: %s\n", t)
		}

		if c := strings.TrimSpace(m.Content); c != "" {
			fmt.Fprintf(&b, "[Assistant]: %s\n", c)
		}

		for _, tc := range m.Calls {
			fmt.Fprintln(&b, tc.ForTranscript())
		}
	case roles.Tool:
		fmt.Fprintf(&b, "  ← %s result:\n", m.ToolName)

		for line := range strings.SplitSeq(strings.TrimSpace(m.Content), "\n") {
			fmt.Fprintf(&b, "    %s\n", line)
		}

	case roles.User:
		fmt.Fprintf(&b, "[User]: %s\n", strings.TrimSpace(m.Content))

	case roles.System:
		fmt.Fprintf(&b, "[System]: %s\n", strings.TrimSpace(m.Content))
	}

	return b.String()
}

// ForLog returns a formatted string for log file output.
func (m Message) ForLog() string {
	if m.IsBookmark() {
		return fmt.Sprintf("[bookmark] --- %s ---", strings.TrimSpace(m.Content))
	}

	if m.IsMetadata() {
		return "[metadata] " + m.Content
	}

	var b strings.Builder

	switch m.Role {
	case roles.Tool:
		fmt.Fprintf(&b, "[tool] %s: %s", m.ToolName, m.Content)
	case roles.Assistant:
		if m.Thinking != "" {
			fmt.Fprintf(&b, "[assistant (thinking)] %s\n", strings.TrimSpace(m.Thinking))
		}

		if m.Content != "" {
			fmt.Fprintf(&b, "[assistant (content)] %s\n", strings.TrimSpace(m.Content))
		}

		for _, tc := range m.Calls {
			fmt.Fprintln(&b, tc.ForLog())
		}
	default:
		fmt.Fprintf(&b, "[%s] %s", m.Role, strings.TrimSpace(m.Content))
	}

	return b.String()
}

// ForTranscript returns a compact representation for compaction transcripts.
// Tool result content is included as-is; callers should pre-truncate if needed.
func (m Message) ForTranscript() string {
	if m.IsBookmark() || m.IsMetadata() {
		return ""
	}

	var b strings.Builder

	switch m.Role {
	case roles.User:
		fmt.Fprintf(&b, "[User]: %s", strings.TrimSpace(m.Content))
	case roles.Assistant:
		if c := strings.TrimSpace(m.Content); c != "" {
			fmt.Fprintf(&b, "[Assistant]: %s\n", c)
		}

		for _, tc := range m.Calls {
			fmt.Fprintln(&b, tc.ForTranscript())
		}
	case roles.Tool:
		fmt.Fprintf(&b, "  ← %s result:\n    %s", m.ToolName, strings.TrimSpace(m.Content))
	case roles.System:
		// Skip system messages in transcripts
	}

	return b.String()
}

// Render converts the messages into a human-readable text representation.
func (m Messages) Render() string {
	var b strings.Builder

	for _, msg := range m {
		b.WriteString(msg.Render())
	}

	return b.String()
}
