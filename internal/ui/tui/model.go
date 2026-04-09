package tui

import (
	"log"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/spintext"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/internal/ui/tui/autocomplete"
	"github.com/idelchi/aura/pkg/llm/message"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// State represents the current UI state.
type State int

const (
	// StateInput indicates the UI is accepting user input.
	StateInput State = iota
	// StateStreaming indicates the UI is receiving streamed content.
	StateStreaming
)

// Layout constants for the TUI.
const (
	textareaPadding  = 4 // Left/right padding for textarea
	inputBorderSize  = 2 // Top + bottom border
	layoutPadding    = 2 // Extra vertical padding
	footerHeight     = 2 // Status line + help line below input
	maxTextareaLines = 3 // Maximum lines before textarea scrolls
)

// selectionState holds text selection fields for mouse-driven copy.
type selectionState struct {
	viewportTop int      // Terminal row where viewport starts (for mouse coordinate translation)
	start       Position // Start of selection
	end         Position // End of selection
	active      bool     // Currently dragging
	enabled     bool     // Has an active selection
}

// exitState holds the double-Ctrl+C exit confirmation fields.
type exitState struct {
	ctrlCPending bool      // True after first Ctrl+C when empty
	ctrlCTime    time.Time // When the warning was shown (auto-clear after timeout)
}

// flashState holds the transient footer notification fields.
type flashState struct {
	message string   // Current flash text (empty = hidden)
	level   ui.Level // Severity for styling
	gen     int      // Monotonic counter — prevents stale timer from clearing newer flash
}

// clearFlashMsg is sent by a tea.Tick to auto-clear a flash after its TTL.
type clearFlashMsg struct {
	gen int
}

// interactionState holds ask + confirm dialog fields.
type interactionState struct {
	askResponse     chan<- string           // Response channel for pending ask (nil = no ask active)
	askOptions      []ui.AskOption          // Options for the current ask
	askMulti        bool                    // Whether multi-select is enabled
	confirmResponse chan<- ui.ConfirmAction // nil = no confirmation pending
}

// Model holds the TUI state.
type Model struct {
	status   ui.Status            // Status bar information
	hints    ui.DisplayHints      // UI display preferences
	state    State                // Current UI state
	textarea textarea.Model       // User input area
	viewport viewport.Model       // Scrollable chat history
	spinner  spinner.Model        // Animated spinner for streaming state
	spintext *spintext.SpinText   // Randomized spinner messages
	input    chan<- ui.UserInput  // Channel for sending user input
	actions  chan<- ui.UserAction // Channel for sending actions
	cancel   chan<- struct{}      // Channel for sending cancel requests
	width    int                  // Terminal width
	height   int                  // Terminal height

	// Conversation history
	messages       message.Messages // All finalized messages
	currentMessage *message.Message // Message being streamed

	// Spinner message for current streaming session
	spinnerMsg string

	// Latest streaming tool output line (shown below spinner during execution)
	toolOutputLine string

	// Scroll state
	scrollLocked bool // When true, don't auto-scroll during streaming

	// Input history
	hist *history // Persistent file-backed input history with navigation

	// Pending injections
	pendingMessages []string // Messages queued but not yet processed by backend

	// Text selection
	selection selectionState

	// Exit confirmation
	exit exitState

	// Flash message
	flash flashState

	// Slash command hints
	hintFunc HintFunc // Resolves slash command name to hint text

	// Directive autocomplete
	completer *autocomplete.Completer // Directive name + path autocomplete

	// Model picker overlay
	picker *Picker // Active picker (nil = hidden)

	// Scrollable pager overlay
	pager *Pager // Active pager for multi-line command output (nil = hidden)

	// Diff preview confirm overlay
	confirmPager *ConfirmPager // Active diff confirm (nil = hidden)

	// Ask + confirm interaction
	interaction interactionState
}

// NewModel creates a new TUI model.
func NewModel(
	status ui.Status,
	input chan<- ui.UserInput,
	actions chan<- ui.UserAction,
	cancel chan<- struct{},
	historyPath string,
	hintFunc HintFunc,
	workdir string,
) Model {
	// Setup textarea for input
	ta := textarea.New()

	ta.Placeholder = "Type your message..."
	ta.Focus()

	ta.CharLimit = 0 // No limit
	ta.ShowLineNumbers = false
	ta.SetHeight(1) // Start with 1 line, will grow to max 3

	ta.MaxHeight = 3 // Max 3 lines before scrolling

	// Setup viewport for chat history
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.SetContent("")

	// Setup spinner for streaming state
	sp := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205"))),
	)

	// Load persisted input history
	h := newHistory(historyPath, 1000)
	if err := h.Load(); err != nil {
		log.Printf("warning: failed to load history: %v", err)
	}

	// Create directive autocomplete if workdir is set
	var comp *autocomplete.Completer

	if workdir != "" {
		comp = autocomplete.New(workdir)
	}

	return Model{
		status:          status,
		state:           StateInput,
		textarea:        ta,
		viewport:        vp,
		spinner:         sp,
		spintext:        spintext.Default(),
		input:           input,
		actions:         actions,
		cancel:          cancel,
		width:           80,
		height:          24,
		messages:        message.Messages{},
		hist:            h,
		pendingMessages: []string{},
		hintFunc:        hintFunc,
		completer:       comp,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

// setFlash shows a transient message in the footer area.
// Non-error levels auto-clear after 5 seconds via a generation-guarded timer.
func (m *Model) setFlash(text string, level ui.Level) tea.Cmd {
	m.flash.message = text
	m.flash.level = level
	m.flash.gen++
	m.recalculateLayout()

	if level == ui.LevelError {
		return nil // errors persist until dismissed
	}

	gen := m.flash.gen

	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return clearFlashMsg{gen: gen}
	})
}

// clearFlash removes any active flash message.
func (m *Model) clearFlash() {
	if m.flash.message != "" {
		m.flash.message = ""
		m.recalculateLayout()
	}
}

// currentHint returns the hint text and flush mode for the current input.
// Supports slash command hints (/command, flush=false), directive autocomplete (@File[, flush=true),
// and history suggestions (flush=true). Priority: slash hints > directives > history.
// flush=true means the hint overwrites the cursor cell (no gap); flush=false preserves the cursor gap.
func (m Model) currentHint() (string, bool) {
	text := m.textarea.Value()

	// Slash command hints (not flush — cursor gap preserved)
	if m.hintFunc != nil && strings.HasPrefix(text, "/") {
		fields := strings.Fields(text)
		if len(fields) == 1 && !strings.HasSuffix(text, " ") {
			if hint := m.hintFunc(fields[0]); hint != "" {
				return hint, false
			}
		}
	}

	// Directive autocomplete (flush — no cursor gap)
	if m.completer != nil {
		if hint := m.completer.Complete(text, len(text)); hint != "" {
			return hint, true
		}
	}

	// History suggestion (flush — lowest priority)
	if match := m.hist.Suggest(text); match != "" {
		return match[len(text):], true
	}

	return "", false
}

// finishStreaming finalizes the streaming state and returns to input state.
func (m *Model) finishStreaming() tea.Cmd {
	// Finalize current message if any
	if m.currentMessage != nil {
		m.messages = append(m.messages, *m.currentMessage)
		m.currentMessage = nil
	}

	m.updateViewportContent()

	m.state = StateInput
	m.textarea.Focus()

	m.scrollLocked = false
	m.spinnerMsg = ""
	m.toolOutputLine = ""
	m.viewport.GotoBottom()

	return textarea.Blink
}
