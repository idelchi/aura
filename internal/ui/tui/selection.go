package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/pkg/clipboard"

	tea "charm.land/bubbletea/v2"
)

// Position represents a point in the viewport (column and line).
type Position struct {
	Col  int // Column position
	Line int // Line position
}

// startSelection begins a new text selection at the given position.
func (m *Model) startSelection(col, line int) {
	m.selection.start = Position{Col: col, Line: line}
	m.selection.end = Position{Col: col, Line: line}
	m.selection.active = true
	m.selection.enabled = true
}

// extendSelection updates the end of the current selection.
func (m *Model) extendSelection(col, line int) {
	m.selection.end = Position{Col: col, Line: line}
}

// endSelection marks the selection as inactive (dragging complete).
func (m *Model) endSelection() {
	m.selection.active = false
}

// clearSelection removes the current selection.
func (m *Model) clearSelection() {
	m.selection.active = false
	m.selection.enabled = false
	m.selection.start = Position{Col: 0, Line: 0}
	m.selection.end = Position{Col: 0, Line: 0}
}

// hasSelection returns true if there's a non-empty selection.
func (m *Model) hasSelection() bool {
	return m.selection.enabled && (m.selection.start != m.selection.end)
}

// getSelectedText extracts the text within the current selection.
func (m *Model) getSelectedText() string {
	if !m.hasSelection() {
		return ""
	}

	content := m.renderChatHistory()
	lines := strings.Split(content, "\n")

	// Normalize selection bounds
	start := m.selection.start
	end := m.selection.end

	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start
	}

	// Clamp to content bounds
	if start.Line < 0 {
		start.Line = 0
		start.Col = 0
	}

	if end.Line >= len(lines) {
		end.Line = len(lines) - 1
		if end.Line >= 0 {
			end.Col = ansi.StringWidth(lines[end.Line])
		}
	}

	var result strings.Builder

	if start.Line == end.Line {
		if start.Line >= len(lines) {
			return ""
		}

		line := lines[start.Line]
		lineWidth := ansi.StringWidth(line)
		startCol := min(start.Col, lineWidth)
		endCol := min(end.Col, lineWidth)

		if startCol >= endCol {
			return ""
		}

		// Cut by visual position
		result.WriteString(ansi.Cut(line, startCol, endCol))
	} else {
		// Multi-line selection
		for i := start.Line; i <= end.Line && i < len(lines); i++ {
			line := lines[i]
			lineWidth := ansi.StringWidth(line)

			switch i {
			case start.Line:
				startCol := min(start.Col, lineWidth)
				result.WriteString(ansi.Cut(line, startCol, lineWidth))
			case end.Line:
				endCol := min(end.Col, lineWidth)
				result.WriteString(ansi.Cut(line, 0, endCol))
			default:
				result.WriteString(line)
			}

			if i < end.Line {
				result.WriteString("\n")
			}
		}
	}

	// Strip ANSI from the final extracted text
	return ansi.Strip(result.String())
}

// applySelectionToLine applies selection styling to the selected portion of a line.
// Returns the styled line with selection highlighting applied.
func (m Model) applySelectionToLine(line string, lineNum int) string {
	if !m.selection.enabled {
		return line
	}

	start, end := m.selection.start, m.selection.end
	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start
	}

	// Check if this line is within the selection
	if lineNum < start.Line || lineNum > end.Line {
		return line
	}

	lineWidth := ansi.StringWidth(line)
	selStart := 0
	selEnd := lineWidth

	if lineNum == start.Line {
		selStart = start.Col
	}

	if lineNum == end.Line {
		selEnd = end.Col
	}

	// Clamp to visual bounds
	selStart = max(0, min(selStart, lineWidth))
	selEnd = max(0, min(selEnd, lineWidth))

	// If no selection on this line, return as-is
	if selStart >= selEnd {
		return line
	}

	// Use ansi.Cut for visual-width-aware slicing
	var result strings.Builder

	if selStart > 0 {
		result.WriteString(ansi.Cut(line, 0, selStart))
	}

	result.WriteString(selectionStyle.Render(ansi.Strip(ansi.Cut(line, selStart, selEnd))))

	if selEnd < lineWidth {
		result.WriteString(ansi.Cut(line, selEnd, lineWidth))
	}

	return result.String()
}

// copySelectionToClipboard copies the selected text to clipboard using OSC 52.
// Captures text NOW before selection is cleared.
func (m *Model) copySelectionToClipboard() tea.Cmd {
	text := m.getSelectedText()
	if text == "" {
		return nil
	}

	return func() tea.Msg {
		clipboard.Copy(text)

		return nil
	}
}
