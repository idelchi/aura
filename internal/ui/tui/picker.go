package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/idelchi/aura/internal/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Picker is an interactive overlay for selecting an item from a grouped list.
type Picker struct {
	title  string
	items  []ui.PickerItem
	cursor int    // index into selectable (non-header) entries
	filter string // type-to-filter text
	width  int
	height int
	offset int // scroll offset for visible window
}

// Picker styles.
var (
	pickerHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Faint(true)

	pickerCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	pickerItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	pickerCurrentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214"))

	pickerFilterStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39"))

	pickerTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	pickerFooterStyle = lipgloss.NewStyle().
				Faint(true).
				Foreground(lipgloss.Color("241"))
)

// NewPicker creates a picker from the given items.
// The cursor starts on the current item (if any).
func NewPicker(title string, items []ui.PickerItem, width, height int) *Picker {
	p := &Picker{
		title:  title,
		items:  items,
		width:  width,
		height: height,
	}

	for i, item := range items {
		if item.Current {
			p.cursor = i

			break
		}
	}

	p.ensureVisible()

	return p
}

// HandleKey processes a key event.
// Returns: handled (always true), selected item (if Enter), dismissed (if Esc).
func (p *Picker) HandleKey(msg tea.KeyPressMsg) (selected *ui.PickerItem, dismissed bool) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return nil, true

	case "enter":
		selectable := p.selectableItems()
		if len(selectable) > 0 && p.cursor < len(selectable) {
			item := selectable[p.cursor]

			return &item, false
		}

		return nil, false

	case "up":
		selectable := p.selectableItems()
		if len(selectable) > 0 {
			p.cursor = (p.cursor - 1 + len(selectable)) % len(selectable)
			p.ensureVisible()
		}

	case "down":
		selectable := p.selectableItems()
		if len(selectable) > 0 {
			p.cursor = (p.cursor + 1) % len(selectable)
			p.ensureVisible()
		}

	case "backspace":
		if len(p.filter) > 0 {
			p.filter = p.filter[:len(p.filter)-1]
			p.cursor = 0
			p.offset = 0
		}

	case "ctrl+u":
		p.filter = ""
		p.cursor = 0
		p.offset = 0

	default:
		// Type-to-filter: append printable runes
		if len(msg.Text) > 0 && unicode.IsPrint(rune(msg.Text[0])) {
			p.filter += msg.Text

			p.cursor = 0
			p.offset = 0
		}
	}

	return nil, false
}

// View renders the picker overlay.
func (p *Picker) View() string {
	var b strings.Builder

	// Title
	title := p.title
	if p.filter != "" {
		title = fmt.Sprintf("%s (filter: %s)", p.title, pickerFilterStyle.Render(p.filter))
	}

	b.WriteString(pickerTitleStyle.Render(title))
	b.WriteString("\n")

	// Build display entries
	entries := p.buildEntries()

	// Available height for items (minus title line and footer line)
	available := max(p.height-2, 1)

	// Apply scroll window
	start := p.offset
	end := min(start+available, len(entries))

	for i := start; i < end; i++ {
		b.WriteString(entries[i])
		b.WriteString("\n")
	}

	// Pad remaining lines to fill viewport
	rendered := end - start
	for i := rendered; i < available; i++ {
		b.WriteString("\n")
	}

	// Footer
	footer := "↑↓ navigate • enter select • esc cancel"

	if p.filter != "" {
		footer += " • ctrl+u clear filter"
	}

	b.WriteString(pickerFooterStyle.Render(footer))

	return b.String()
}

// selectableItems returns filtered, selectable items.
func (p *Picker) selectableItems() []ui.PickerItem {
	var result []ui.PickerItem

	filter := strings.ToLower(p.filter)

	for _, item := range p.items {
		if item.Disabled {
			continue
		}

		if filter != "" &&
			!strings.Contains(strings.ToLower(item.Label), filter) &&
			!strings.Contains(strings.ToLower(item.Description), filter) {
			continue
		}

		result = append(result, item)
	}

	return result
}

// buildEntries builds the display lines with group headers and selectable items.
func (p *Picker) buildEntries() []string {
	filter := strings.ToLower(p.filter)
	selectable := p.selectableItems()

	var lines []string

	selectIdx := 0 // tracks position in selectable list
	lastGroup := ""

	for _, item := range p.items {
		// Disabled items render as dimmed headers
		if item.Disabled {
			header := fmt.Sprintf("── %s ──", item.Group)

			lines = append(lines, pickerHeaderStyle.Render(header))

			continue
		}

		if filter != "" &&
			!strings.Contains(strings.ToLower(item.Label), filter) &&
			!strings.Contains(strings.ToLower(item.Description), filter) {
			continue
		}

		// Insert group header when group changes
		if item.Group != "" && item.Group != lastGroup {
			lastGroup = item.Group

			header := fmt.Sprintf("── %s ──", item.Group)

			lines = append(lines, pickerHeaderStyle.Render(header))
		}

		// Build item line
		marker := "  "
		style := pickerItemStyle

		if selectIdx == p.cursor {
			marker = "> "
			style = pickerCursorStyle
		}

		name := item.Label
		if item.Current {
			name += " *"

			if selectIdx != p.cursor {
				style = pickerCurrentStyle
			}
		}

		line := marker + style.Render(name+item.Icons)
		if item.Description != "" {
			line += pickerHeaderStyle.Render(" - " + item.Description)
		}

		lines = append(lines, line)

		selectIdx++
	}

	if len(selectable) == 0 {
		lines = append(lines, pickerHeaderStyle.Render("  no matches"))
	}

	return lines
}

// ensureVisible adjusts scroll offset so the cursor is within the visible window.
func (p *Picker) ensureVisible() {
	// We need to find the display line index of the current cursor item.
	// This is approximate since headers add extra lines, but good enough.
	entries := p.buildEntries()
	available := max(p.height-2, 1)

	// Find the line index of the cursor item in entries
	cursorLine := p.findCursorLine(entries)

	if cursorLine < p.offset {
		p.offset = cursorLine
	}

	if cursorLine >= p.offset+available {
		p.offset = cursorLine - available + 1
	}
}

// findCursorLine finds the display line index of the currently selected item.
func (p *Picker) findCursorLine(entries []string) int {
	for i, line := range entries {
		if strings.HasPrefix(line, "> ") {
			return i
		}
	}

	return 0
}
