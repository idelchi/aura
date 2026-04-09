package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// visualLineCount returns the number of visual lines the textarea content
// occupies, accounting for soft wrapping at the textarea's width.
func (m *Model) visualLineCount() int {
	width := m.textarea.Width()
	if width <= 0 {
		return 1
	}

	total := 0

	for line := range strings.SplitSeq(m.textarea.Value(), "\n") {
		w := ansi.StringWidth(line)
		if w <= width {
			total++
		} else {
			total += (w + width - 1) / width
		}
	}

	return max(1, total)
}

// adjustTextareaHeight sets textarea height based on visual line count, up to max 3 lines.
func (m *Model) adjustTextareaHeight() {
	lines := m.visualLineCount()
	targetHeight := max(1, min(lines, maxTextareaLines))

	if m.textarea.Height() == targetHeight {
		return
	}

	m.textarea.SetHeight(targetHeight)
	m.textarea.SetValue(m.textarea.Value())
	m.recalculateLayout()
}

// recalculateLayout recalculates viewport dimensions based on textarea height.
func (m *Model) recalculateLayout() {
	textareaHeight := m.textarea.Height()
	inputHeight := textareaHeight + inputBorderSize

	flashHeight := 0

	if m.flash.message != "" {
		flashHeight = 1
	}

	m.viewport.SetHeight(max(1, m.height-inputHeight-footerHeight-flashHeight-layoutPadding))
	m.viewport.SetWidth(m.width)
}
