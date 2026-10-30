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

	"charm.land/lipgloss/v2"
)

// Labels shown in the chat history.
const (
	userLabel      = "You:"
	assistantLabel = "Aura:"
	thinkingLabel  = "Thinking"
)

// updateViewportContent updates the viewport with current chat history.
// Must be called from Update(), never from View().
func (m *Model) updateViewportContent() {
	m.viewport.SetContent(m.renderChatHistory())
}

// renderChatHistory renders all messages as a string for the viewport.
// Tracks line numbers to support text selection highlighting: currentLine is
// the content line the next write starts on.
func (m Model) renderChatHistory() string {
	var b strings.Builder

	currentLine := 0

	for _, msg := range m.messages {
		rendered, lines := m.renderMessage(msg, m.width, currentLine)
		if rendered == "" {
			// Nothing visible (e.g. only hidden thinking) — no separator either.
			continue
		}

		b.WriteString(rendered)
		b.WriteString("\n\n")

		currentLine += lines + 1 // the separator ends the last line and adds one blank line
	}

	m.renderUserEchoes(&b, &currentLine, m.processingMessages)

	if m.currentMessage == nil {
		m.renderUserEchoes(&b, &currentLine, m.pendingMessages)
	}

	// Progress (spinner, phase, compaction) is shown in the activity row, not in the history:
	// a message without visible parts yet (e.g. only hidden thinking) renders nothing here.
	if m.currentMessage != nil {
		if rendered, lines := m.renderMessage(*m.currentMessage, m.width, currentLine); rendered != "" {
			b.WriteString(rendered)

			currentLine += lines - 1 // no separator: the next write continues on the last line
		}

		m.renderUserEchoes(&b, &currentLine, m.pendingMessages)
	}

	// Show latest streaming tool output line below the message area.
	// Truncated rather than wrapped so the live line never changes the layout height.
	if m.toolOutputLine != "" {
		b.WriteString("\n")
		b.WriteString(toolResultStyle.Render(ansi.Truncate("  "+m.toolOutputLine, m.width, "…")))
	}

	return b.String()
}

func (m Model) renderUserEchoes(b *strings.Builder, currentLine *int, texts []string) {
	for _, text := range texts {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteString("\n\n")

			*currentLine += 2
		}

		msg := message.Message{
			Role:  roles.User,
			Parts: []part.Part{{Type: part.Content, Text: text}},
		}
		rendered, lines := m.renderMessage(msg, m.width, *currentLine)
		b.WriteString(rendered)
		b.WriteString("\n\n")

		*currentLine += lines + 1
	}
}

// renderMessage renders a single message with text wrapping.
// Returns the rendered string and the number of lines rendered.
// A message without visible output renders as ("", 0).
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

	var rendered string

	switch msg.Role {
	case roles.User:
		var text strings.Builder

		for _, p := range msg.Parts {
			if p.IsContent() {
				text.WriteString(strings.TrimSpace(p.Text))
			}
		}

		rendered = renderLabelled(userLabel, userStyle, text.String(), contentStyle, width)

	case roles.System:
		var b strings.Builder

		for _, p := range msg.Parts {
			if p.IsContent() {
				wrapped := ansi.Wordwrap(strings.TrimSpace(p.Text), width, "")
				b.WriteString(syntheticStyle.Render(wrapped))
			}
		}

		rendered = b.String()

	case roles.Assistant:
		rendered = m.renderAssistant(msg, width)
	}

	if rendered == "" {
		return "", 0
	}

	// Apply selection highlighting to each line
	lines := strings.Split(rendered, "\n")

	for i, line := range lines {
		lines[i] = m.applySelectionToLine(line, startLine+i)
	}

	return strings.Join(lines, "\n"), len(lines)
}

// renderAssistant renders the visible parts of an assistant message as blocks separated by one blank line.
// A turn interleaves parts across loop iterations (thinking, content, tools, thinking, ...),
// so every block starts on its own line regardless of which part type precedes it.
// Content opens with the assistant label when it starts the message or follows a tool;
// thinking and tool blocks carry their own headings.
func (m Model) renderAssistant(msg message.Message, width int) string {
	var blocks []string

	// spoken tracks whether content was shown since the message start or the last tool.
	// It ignores thinking so that toggling thinking visibility never moves the assistant label.
	spoken := false

	for _, p := range msg.Parts {
		switch p.Type {
		case part.Thinking:
			text := strings.TrimSpace(p.Text)
			if !m.hints.Verbose || text == "" {
				continue
			}

			blocks = append(blocks, thinkingLabelStyle.Render(thinkingLabel)+"\n"+
				renderLines(thinkingStyle, ansi.Wordwrap(text, width, "")))

		case part.Content:
			text := strings.TrimSpace(p.Text)
			if text == "" {
				continue
			}

			if spoken {
				blocks = append(blocks, renderLines(contentStyle, ansi.Wordwrap(text, width, "")))
			} else {
				blocks = append(blocks, renderLabelled(assistantLabel, assistantStyle, text, contentStyle, width))
			}

			spoken = true

		case part.Tool:
			if p.Call == nil {
				continue
			}

			blocks = append(blocks, m.renderTool(*p.Call, width))

			spoken = false
		}
	}

	if msg.Error != nil {
		var errText string

		// Check if the error is of type context cancellation
		if errors.Is(msg.Error, context.Canceled) {
			errText = "Operation canceled by user"
		} else {
			errText = ansi.Wordwrap(fmt.Sprintf("Error: %v", msg.Error), width, "")
		}

		errBlock := renderLines(errorStyle, errText)

		if len(blocks) == 0 {
			return errBlock
		}

		// The error closes the message directly below the last block.
		return strings.Join(blocks, "\n\n") + "\n" + errBlock
	}

	return strings.Join(blocks, "\n\n")
}

// renderTool renders a tool call heading and, once finished, its result directly below it.
func (m Model) renderTool(tc call.Call, width int) string {
	header := tc.DisplayHeader()

	switch tc.State {
	case call.Running:
		// The spinner frame carries its own style and trailing space.
		return renderHanging(m.spinner.View(), header, toolStyle, width)

	case call.Complete:
		heading := renderHanging(toolStyle.Render("✓ "), header, toolStyle, width)

		if tc.Error != nil {
			return heading + "\n" + renderLines(toolErrorStyle, ansi.Wrap(tc.DisplayResult(), width, ""))
		}

		if result, ok := renderToolResult(&tc, width); ok {
			return heading + "\n" + result
		}

		return heading + "\n" + renderLines(toolResultStyle, ansi.Wrap(tc.DisplayResult(), width, ""))

	case call.Error:
		heading := renderHanging(toolErrorStyle.Render("✗ "), header, toolErrorStyle, width)

		return heading + "\n" + renderLines(toolErrorStyle, ansi.Wrap(tc.DisplayResult(), width, ""))

	default:
		// Pending, including calls restored from history before their results.
		return renderHanging(toolPendingStyle.Render("○ "), header, toolPendingStyle, width)
	}
}

// renderLabelled renders a styled label followed by wrapped text, keeping the first line,
// label included, within width. Continuation lines start flush left.
func renderLabelled(label string, labelStyle lipgloss.Style, text string, textStyle lipgloss.Style, width int) string {
	// Wrap with a placeholder word of the label's width in front, so the first line accounts for it.
	// Wordwrap never breaks inside a word, so the placeholder stays intact on the first line.
	placeholder := strings.Repeat("x", ansi.StringWidth(label))
	wrapped := ansi.Wordwrap(placeholder+" "+text, width, "")

	return labelStyle.Render(label) + renderLines(textStyle, strings.TrimPrefix(wrapped, placeholder))
}

// renderHanging renders an already styled prefix followed by text wrapped to width,
// indenting continuation lines to align under the text.
func renderHanging(prefix, text string, style lipgloss.Style, width int) string {
	indent := ansi.StringWidth(prefix)
	lines := strings.Split(ansi.Wrap(text, max(1, width-indent), ""), "\n")

	for i, line := range lines {
		if i == 0 {
			lines[i] = prefix + style.Render(line)
		} else {
			lines[i] = strings.Repeat(" ", indent) + style.Render(line)
		}
	}

	return strings.Join(lines, "\n")
}

// renderLines styles each line on its own, so lipgloss does not pad lines to a common block width.
func renderLines(style lipgloss.Style, text string) string {
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		lines[i] = style.Render(line)
	}

	return strings.Join(lines, "\n")
}
