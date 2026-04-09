package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Pager styles.
var (
	pagerTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	pagerContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	pagerFooterStyle = lipgloss.NewStyle().
				Faint(true).
				Foreground(lipgloss.Color("241"))
)

// Pager is a scrollable read-only overlay for multi-line command output.
type Pager struct {
	title  string   // custom title (shown in header)
	lines  []string // content split into lines
	offset int      // scroll offset (first visible line)
	width  int
	height int
	raw    bool // when true, skip pagerContentStyle wrapping (content has its own ANSI)
}

// NewPager creates a pager from the given content string.
func NewPager(title, content string, width, height int) *Pager {
	return &Pager{
		title:  title,
		lines:  strings.Split(content, "\n"),
		width:  width,
		height: height,
	}
}

// HandleKey processes a key event.
// Returns true if the pager should be dismissed.
func (p *Pager) HandleKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		return true

	case "up":
		if p.offset > 0 {
			p.offset--
		}

	case "down":
		if p.offset < p.maxOffset() {
			p.offset++
		}

	case "pgup":
		p.offset -= p.contentHeight()
		if p.offset < 0 {
			p.offset = 0
		}

	case "pgdown":
		p.offset += p.contentHeight()
		if p.offset > p.maxOffset() {
			p.offset = p.maxOffset()
		}

	case "home":
		p.offset = 0

	case "end":
		p.offset = p.maxOffset()
	}

	return false
}

// View renders the pager overlay.
func (p *Pager) View() string {
	var b strings.Builder

	// Title
	total := len(p.lines)
	title := fmt.Sprintf("%s (%d lines) — esc/q to close", p.title, total)

	b.WriteString(pagerTitleStyle.Render(title))
	b.WriteString("\n")

	// Content lines
	start := p.offset
	end := min(start+p.contentHeight(), total)

	for i := start; i < end; i++ {
		if p.raw {
			b.WriteString(p.lines[i])
		} else {
			b.WriteString(pagerContentStyle.Render(p.lines[i]))
		}

		b.WriteString("\n")
	}

	// Pad remaining lines to fill viewport
	rendered := end - start
	for i := rendered; i < p.contentHeight(); i++ {
		b.WriteString("\n")
	}

	// Footer with scroll position
	var footer string

	if total <= p.contentHeight() {
		footer = "esc/q close"
	} else {
		pct := 0

		if p.maxOffset() > 0 {
			pct = p.offset * 100 / p.maxOffset()
		}

		footer = fmt.Sprintf("↑↓ scroll • pgup/pgdn page • home/end • esc/q close — %d%%", pct)
	}

	b.WriteString(pagerFooterStyle.Render(footer))

	return b.String()
}

// contentHeight returns the number of lines available for content (minus title and footer).
func (p *Pager) contentHeight() int {
	h := max(p.height-2, 1)

	return h
}

// maxOffset returns the maximum scroll offset.
func (p *Pager) maxOffset() int {
	m := max(len(p.lines)-p.contentHeight(), 0)

	return m
}
