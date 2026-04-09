package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"
)

// ghostStyle styles inline ghost hint text (faint, dim foreground).
var ghostStyle = lipgloss.NewStyle().
	Faint(true).
	Foreground(lipgloss.Color("243"))

// injectGhostHint splices styled ghost text into the padding area of a
// textarea's rendered first line, immediately after the cursor position.
//
// The textarea renders each line as:
//
//	[prompt][typed text][cursor char][padding spaces to fill width]
//
// When flush is true (autocomplete), the hint overwrites the cursor cell so it
// appears immediately after the typed text: "@Fi" + "le[" = "@File[".
// When flush is false (type hint), the cursor gap is preserved: "/drop" + " " + "[n|all]".
func injectGhostHint(rendered, hint string, textWidth, areaWidth, promptWidth int, flush bool) string {
	cursorCells := 1
	contentCells := textWidth + cursorCells
	available := areaWidth - contentCells

	if available <= 0 {
		return rendered
	}

	// Split at first newline to isolate the cursor line.
	// For single-line textareas, there may be no newline at all.
	var firstLine, rest string

	if idx := strings.IndexByte(rendered, '\n'); idx >= 0 {
		firstLine = rendered[:idx]
		rest = rendered[idx:]
	} else {
		firstLine = rendered
		rest = ""
	}

	// For flush mode, the hint also gets the cursor cell
	hintSpace := available

	if flush {
		hintSpace += cursorCells
	}

	// Truncate hint to fit available space
	displayHint := hint
	if ansi.StringWidth(hint) > hintSpace {
		displayHint = ansi.Truncate(hint, max(0, hintSpace-1), "…")
	}

	hintCells := ansi.StringWidth(displayHint)

	// Truncate to prompt + text (+ cursor if not flush)
	truncateAt := promptWidth + textWidth

	if !flush {
		truncateAt += cursorCells
	}

	contentPart := ansi.Truncate(firstLine, truncateAt, "")
	remainingPad := max(0, hintSpace-hintCells)

	return contentPart + ghostStyle.Inline(true).Render(displayHint) + strings.Repeat(" ", remainingPad) + rest
}
