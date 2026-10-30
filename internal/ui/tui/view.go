package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/internal/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Styles for the TUI.
var (
	userStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")) // Blue for user

	assistantStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")) // Orange for assistant

	// Secondary text uses explicit muted colors instead of Faint: terminals render
	// faint inconsistently, and faint on top of dark gray becomes unreadable.

	thinkingLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Italic(true).
				Foreground(lipgloss.Color("246")) // Muted gray heading for thinking

	thinkingStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("246")) // Muted gray italic for thinking

	contentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")) // Light gray for content

	toolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("74")) // Muted blue for tool headings

	toolPendingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")) // Gray for pending tools

	toolResultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")) // Muted gray for results

	toolErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("167")) // Muted red for tool errors

	selectionStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("33")). // Blue background
			Foreground(lipgloss.Color("15"))  // White text

	statusLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")) // Dim gray

	helpLineStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.Color("241"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")). // Red for errors
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")). // Green
			Bold(true)

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")). // Yellow/orange
			Bold(true)

	commandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("249")) // Light gray, visible

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.Color("241"))

	syntheticStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")). // Blue
			Faint(true)

	activityStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")) // Light gray: the assistant is working

	activityAwaitingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")). // Orange: the assistant waits for the user
				Bold(true)

	activityIdleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")) // Muted gray: nothing running

	activityFailedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("167")) // Muted red: the last turn failed
)

// renderActivity renders the one-line activity row shown between the viewport and the input.
// It is outside the viewport, so it stays visible however the history is scrolled.
// Only work is animated; waiting for the user and idle states are static.
func (m Model) renderActivity(now time.Time) string {
	text := m.activity.Display(now)

	var row string

	switch m.activity.State() {
	case ui.ActivityWorking:
		row = m.spinner.View() + activityStyle.Render(text)
	case ui.ActivityAwaitingUser:
		row = activityAwaitingStyle.Render("? " + text)
	case ui.ActivityDone:
		row = activityIdleStyle.Render("✓ " + text)
	case ui.ActivityFailed:
		row = activityFailedStyle.Render("✗ " + text)
	case ui.ActivityCancelled:
		row = activityIdleStyle.Render("■ " + text)
	default:
		row = activityIdleStyle.Render("  " + text)
	}

	return ansi.Truncate(row, m.width, "…")
}

// View renders the TUI.
func (m Model) View() tea.View {
	var b strings.Builder

	// Viewport area: picker overlay, confirm pager, pager overlay, or chat history
	if m.picker != nil {
		b.WriteString(m.picker.View())
	} else if m.confirmPager != nil {
		b.WriteString(m.confirmPager.View())
	} else if m.pager != nil {
		b.WriteString(m.pager.View())
	} else {
		b.WriteString(m.viewport.View())
	}

	b.WriteString("\n")

	// Activity row: always visible, whatever the viewport shows or how far it is scrolled.
	b.WriteString(m.renderActivity(m.clock()))
	b.WriteString("\n")

	// Input area (with inline ghost hint if applicable)
	taView := m.textarea.View()
	if hint, flush := m.currentHint(); hint != "" {
		taView = injectGhostHint(taView, hint,
			ansi.StringWidth(m.textarea.Value()),
			m.textarea.Width(),
			ansi.StringWidth(m.textarea.Prompt),
			flush)
	}

	b.WriteString(inputStyle.Render(taView))
	b.WriteString("\n")

	// Flash message (transient notification)
	if m.flash.message != "" {
		wrapped := ansi.Wordwrap(m.flash.message, m.width, "")

		switch m.flash.level {
		case ui.LevelError:
			b.WriteString(errorStyle.Render(wrapped))
		case ui.LevelSuccess:
			b.WriteString(successStyle.Render(wrapped))
		case ui.LevelWarn:
			b.WriteString(warnStyle.Render(wrapped))
		default:
			b.WriteString(commandStyle.Render(wrapped))
		}

		b.WriteString("\n")
	}

	// Status line (left-aligned)
	statusText := m.status.StatusLine(m.hints)
	b.WriteString(statusLineStyle.Render(statusText))
	b.WriteString("\n")

	// Help lines
	helpLines := []string{
		"Enter: send • Ctrl+C: copy/clear/quit • Ctrl+O: output • ESC: cancel",
		"Ctrl+T: thinking • Ctrl+R: think • Ctrl+E: cycle think • Ctrl+A: auto • Ctrl+S: sandbox",
	}

	if len(m.pendingMessages) > 0 {
		helpLines[0] += fmt.Sprintf(" • (%d pending)", len(m.pendingMessages))
	}

	for _, line := range helpLines {
		padding := m.width - lipgloss.Width(line)
		if padding > 0 {
			b.WriteString(strings.Repeat(" ", padding))
		}

		b.WriteString(helpLineStyle.Render(line))
		b.WriteString("\n")
	}

	v := tea.NewView(b.String())

	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	return v
}
