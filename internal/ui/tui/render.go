package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// updateViewportContent updates the viewport with current chat history.
// Must be called from Update(), never from View().
func (m *Model) updateViewportContent() {
	m.viewport.SetContent(m.renderChatHistory())
}

// renderChatHistory renders all messages as a string for the viewport.
// Tracks line numbers to support text selection highlighting.
func (m Model) renderChatHistory() string {
	var b strings.Builder

	currentLine := 0

	for _, msg := range m.messages {
		rendered, lines := m.renderMessage(msg, m.width, currentLine)
		b.WriteString(rendered)
		b.WriteString("\n\n")

		currentLine += lines + 2 // +2 for the double newline separator
	}

	if m.currentMessage != nil {
		if len(m.currentMessage.Parts) == 0 {
			// No content yet - show spinner placeholder in chat area
			b.WriteString(m.spinner.View() + " " + m.spinnerMsg)
		} else {
			rendered, _ := m.renderMessage(*m.currentMessage, m.width, currentLine)
			b.WriteString(rendered)
		}
	} else if m.spinnerMsg != "" {
		// Standalone spinner (not tied to streaming) — shows during compaction, etc.
		b.WriteString(m.spinner.View() + " " + m.spinnerMsg)
	}

	// Show latest streaming tool output line below the spinner/message area.
	if m.toolOutputLine != "" {
		b.WriteString("\n")
		b.WriteString(toolResultStyle.Render("  " + m.toolOutputLine))
	}

	return b.String()
}

// renderMessage renders a single message with text wrapping.
// Returns the rendered string and the number of lines rendered.
// The startLine parameter indicates the line number where this message begins.
func (m Model) renderMessage(msg message.Message, width, startLine int) (string, int) {
	// Type-specific rendering takes priority over role.
	if msg.IsBookmark() {
		label := strings.TrimSpace(msg.Content)
		divider := syntheticStyle.Render(fmt.Sprintf("--- %s ---", label))

		return divider, 1
	}

	if msg.IsDisplayOnly() {
		wrapped := ansi.Wordwrap(strings.TrimSpace(msg.Content), width, "")
		rendered := syntheticStyle.Render(wrapped)
		lines := strings.Split(rendered, "\n")

		return rendered, len(lines)
	}

	var b strings.Builder

	switch msg.Role {
	case roles.User:
		b.WriteString(userStyle.Render("You: "))
		// Find content parts
		for _, p := range msg.Parts {
			if p.IsContent() {
				wrapped := ansi.Wordwrap(strings.TrimSpace(p.Text), width, "")
				b.WriteString(contentStyle.Render(wrapped))
			}
		}

	case roles.System:
		for _, p := range msg.Parts {
			if p.IsContent() {
				wrapped := ansi.Wordwrap(strings.TrimSpace(p.Text), width, "")
				b.WriteString(syntheticStyle.Render(wrapped))
			}
		}

	case roles.Assistant:
		// Check if there's any visible content to display
		hasVisibleContent := false

		for _, p := range msg.Parts {
			if p.IsContent() || p.IsTool() {
				hasVisibleContent = true

				break
			}

			if p.IsThinking() && m.hints.Verbose {
				hasVisibleContent = true

				break
			}
		}

		if hasVisibleContent {
			b.WriteString(assistantStyle.Render("Aura: "))
		}

		first := true
		prevWasTool := false

		for _, p := range msg.Parts {
			// Add blank line separation between different part types.
			// Only insert separator when a previous visible part exists.
			if !first {
				if p.IsThinking() {
					// No separator before thinking (inline with prefix)
				} else {
					b.WriteString("\n\n") // Blank line for readability
				}
			}

			switch p.Type {
			case part.Thinking:
				if !m.hints.Verbose {
					// Skip hidden thinking without updating first — preserves
					// separator logic between the surrounding visible parts.
					continue
				}

				wrapped := ansi.Wordwrap(strings.TrimSpace(p.Text), width, "")
				b.WriteString(thinkingStyle.Render(wrapped))

				first = false
				prevWasTool = false
			case part.Content:
				// Add "Aura:" prefix if this content follows a tool
				if prevWasTool {
					b.WriteString(assistantStyle.Render("Aura: "))
				}

				wrapped := ansi.Wordwrap(strings.TrimSpace(p.Text), width, "")
				b.WriteString(contentStyle.Render(wrapped))

				first = false
				prevWasTool = false
			case part.Tool:
				tc := p.Call

				// Separator before tool header — raw newlines outside Render()
				// so lipgloss cannot absorb them into styled output.
				b.WriteString("\n\n")

				switch tc.State {
				case call.Pending:
					b.WriteString(toolPendingStyle.Render("○ " + tc.DisplayHeader()))

				case call.Running:
					b.WriteString(toolStyle.Render(m.spinner.View() + " " + tc.DisplayHeader()))

				case call.Complete:
					b.WriteString(toolStyle.Render("✓ " + tc.DisplayHeader()))
					b.WriteString("\n")

					if tc.Error != nil {
						b.WriteString(toolErrorStyle.Render(ansi.Wordwrap(tc.DisplayResult(), width, "")))
					} else if result, ok := renderToolResult(tc, width); ok {
						b.WriteString(result)
					} else {
						b.WriteString(toolResultStyle.Render(ansi.Wordwrap(tc.DisplayResult(), width, "")))
					}

					prevWasTool = true

				case call.Error:
					b.WriteString(toolErrorStyle.Render("✗ " + tc.DisplayHeader()))
					b.WriteString("\n")
					b.WriteString(toolErrorStyle.Render(ansi.Wordwrap(tc.DisplayResult(), width, "")))

					prevWasTool = true
				}

				first = false
			}
		}

		// Error
		if msg.Error != nil {
			b.WriteString("\n")
			// Check if the error is of type context cancellation
			if errors.Is(msg.Error, context.Canceled) {
				b.WriteString(errorStyle.Render("Operation canceled by user"))
			} else {
				errText := ansi.Wordwrap(fmt.Sprintf("Error: %v", msg.Error), width, "")
				b.WriteString(errorStyle.Render(errText))
			}
		}
	}

	// Apply selection highlighting to each line
	rendered := b.String()
	lines := strings.Split(rendered, "\n")

	for i, line := range lines {
		lines[i] = m.applySelectionToLine(line, startLine+i)
	}

	rendered = strings.Join(lines, "\n")

	return rendered, len(lines)
}
