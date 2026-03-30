package tui

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ConfirmPager styles.
var (
	confirmFooterStyle = lipgloss.NewStyle().
				Faint(true).
				Foreground(lipgloss.Color("241"))

	confirmKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)
)

// ConfirmPager is a scrollable diff pager with embedded confirm keybindings.
// Shown instead of the bare Picker when a tool confirmation includes a diff preview.
type ConfirmPager struct {
	pager    *Pager
	response chan<- ui.ConfirmAction
	pattern  string
	width    int
}

// NewConfirmPager creates a ConfirmPager from highlighted diff content.
func NewConfirmPager(title, diff, pattern string, response chan<- ui.ConfirmAction, width, height int) *ConfirmPager {
	// Reserve 1 line for the confirm footer (pager already reserves title + its own footer)
	pager := NewPager(title, diff, width, height-1)

	pager.raw = true

	return &ConfirmPager{
		pager:    pager,
		response: response,
		pattern:  pattern,
		width:    width,
	}
}

// HandleKey processes key events. Returns (action, dismissed).
// action is non-nil when the user picks a confirm action.
// dismissed is true when the user cancels (esc/ctrl+c).
func (c *ConfirmPager) HandleKey(msg tea.KeyPressMsg) (action *ui.ConfirmAction, dismissed bool) {
	switch msg.String() {
	case "a":
		a := ui.ConfirmAllow

		return &a, false
	case "s":
		a := ui.ConfirmAllowSession

		return &a, false
	case "p":
		a := ui.ConfirmAllowPatternProject

		return &a, false
	case "g":
		a := ui.ConfirmAllowPatternGlobal

		return &a, false
	case "d":
		a := ui.ConfirmDeny

		return &a, false
	case "esc", "ctrl+c":
		a := ui.ConfirmDeny

		return &a, true
	default:
		// Forward scrolling keys to the inner pager
		c.pager.HandleKey(msg)

		return nil, false
	}
}

// View renders the confirm pager with diff content and keybinding footer.
func (c *ConfirmPager) View() string {
	var b strings.Builder

	b.WriteString(c.pager.View())
	b.WriteString("\n")

	// Confirm key hints
	footer := fmt.Sprintf(
		"%s Allow  %s Session  %s Project  %s Global  %s Deny  │  ↑↓ scroll  │  esc cancel",
		confirmKeyStyle.Render("[a]"),
		confirmKeyStyle.Render("[s]"),
		confirmKeyStyle.Render("[p]"),
		confirmKeyStyle.Render("[g]"),
		confirmKeyStyle.Render("[d]"),
	)

	b.WriteString(confirmFooterStyle.Render(footer))

	return b.String()
}
