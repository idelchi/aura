package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"

	tea "charm.land/bubbletea/v2"
)

// ctrlCTimeoutMsg signals the Ctrl+C exit warning should be cleared.
type ctrlCTimeoutMsg struct{}

// handleKeyMsg processes keyboard input.
// Returns (handled, cmd) where handled indicates if the key was consumed.
func (m *Model) handleKeyMsg(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	// Pager intercepts all keys when active
	if m.pager != nil {
		return m.handlePagerKey(msg)
	}

	// ConfirmPager intercepts all keys when active
	if m.confirmPager != nil {
		return m.handleConfirmPagerKey(msg)
	}

	// Picker intercepts all keys when active
	if m.picker != nil {
		return m.handlePickerKey(msg)
	}

	// Ask mode (no picker): esc dismisses the question
	if m.interaction.askResponse != nil && msg.String() == "esc" {
		m.interaction.askResponse <- ""

		m.clearAskState()

		return true, nil
	}

	// Reset exit confirmation on any other key
	if m.exit.ctrlCPending && msg.String() != "ctrl+c" {
		m.exit.ctrlCPending = false
		m.textarea.Placeholder = "Type your message..."
	}

	switch msg.String() {
	case "ctrl+c":
		return m.handleCtrlC()

	case "esc":
		if m.state == StateStreaming {
			go func() {
				m.cancel <- struct{}{}
			}()

			return true, nil
		}

	case "alt+enter":
		m.textarea.InsertRune('\n')
		m.adjustTextareaHeight()

		return true, nil

	case "enter":
		return m.handleEnter()

	case "up":
		if m.textarea.Line() == 0 {
			return m.handleHistoryUp()
		}

	case "down":
		if m.textarea.Line() == m.textarea.LineCount()-1 {
			return m.handleHistoryDown()
		}

	case "pgup", "pgdown":
		var cmd tea.Cmd

		m.viewport, cmd = m.viewport.Update(msg)
		m.scrollLocked = !m.viewport.AtBottom()

		return true, cmd

	case "tab":
		// Accept completion when cursor is at end of text
		if m.textarea.Line() == m.textarea.LineCount()-1 {
			text := m.textarea.Value()
			li := m.textarea.LineInfo()

			if li.ColumnOffset+1 >= li.Width {
				// Directive completion (highest priority)
				if m.completer != nil {
					if newText, _, ok := m.completer.Accept(text, len(text)); ok {
						m.textarea.Reset()
						m.textarea.SetValue(newText)

						return true, nil
					}
				}

				// History suggestion
				if match := m.hist.Suggest(text); match != "" {
					m.textarea.Reset()
					m.textarea.SetValue(match)

					return true, nil
				}
			}
		}

		// No completion — cycle mode
		go func() {
			m.actions <- ui.UserAction{Action: ui.NextMode{}}
		}()

		return true, nil

	case "shift+tab":
		go func() {
			m.actions <- ui.UserAction{Action: ui.NextAgent{}}
		}()

		return true, nil

	case "ctrl+t":
		m.hints.Verbose = !m.hints.Verbose
		m.updateViewportContent()
		m.viewport.GotoBottom()

		go func() {
			m.actions <- ui.UserAction{Action: ui.ToggleVerbose{}}
		}()

		return true, nil

	case "ctrl+r":
		go func() {
			m.actions <- ui.UserAction{Action: ui.ToggleThink{}}
		}()

		return true, nil

	case "ctrl+e":
		go func() {
			m.actions <- ui.UserAction{Action: ui.CycleThink{}}
		}()

		return true, nil

	case "ctrl+a":
		go func() {
			m.actions <- ui.UserAction{Action: ui.ToggleAuto{}}
		}()

		return true, nil

	case "ctrl+s":
		go func() {
			m.actions <- ui.UserAction{Action: ui.ToggleSandbox{}}
		}()

		return true, nil

	case "ctrl+o":
		tc := m.lastCompletedToolCall()
		if tc == nil || tc.FullOutput == "" {
			return true, m.setFlash("No tool output to view", ui.LevelInfo)
		}

		title := fmt.Sprintf("[Tool: %s] full output", tc.Name)
		content := tc.FullOutput
		raw := false

		switch tc.Name {
		case "Read":
			if path, ok := tc.Arguments["path"].(string); ok {
				if h := highlightReadSyntax(content, path); h != "" {
					content = h
					raw = true
				}
			}
		case "Rg":
			if pat, ok := tc.Arguments["pattern"].(string); ok {
				if h := highlightRgMatches(content, pat); h != "" {
					content = h
					raw = true
				}
			}
		}

		p := NewPager(title, content, m.width, m.viewport.Height())

		p.raw = raw
		m.pager = p

		return true, nil

	case "right":
		// Accept completion only when cursor is at end of text (last line, last column)
		if m.textarea.Line() == m.textarea.LineCount()-1 {
			text := m.textarea.Value()
			li := m.textarea.LineInfo()

			// Width includes the cursor cell, so cursor-at-end is ColumnOffset+1 >= Width
			if li.ColumnOffset+1 >= li.Width {
				// Directive completion (highest priority)
				if m.completer != nil {
					if newText, _, ok := m.completer.Accept(text, len(text)); ok {
						m.textarea.Reset()
						m.textarea.SetValue(newText)

						return true, nil
					}
				}

				// History suggestion
				if match := m.hist.Suggest(text); match != "" {
					m.textarea.Reset()
					m.textarea.SetValue(match)

					return true, nil
				}
			}
		}
	}

	return false, nil
}

// handleCtrlC processes Ctrl+C with priority: ask dismiss > copy > clear > quit.
func (m *Model) handleCtrlC() (bool, tea.Cmd) {
	// Priority 0: Dismiss pending confirm
	if m.interaction.confirmResponse != nil {
		if m.picker != nil {
			m.picker = nil
		}

		if m.confirmPager != nil {
			m.confirmPager = nil
		}

		m.interaction.confirmResponse <- ui.ConfirmDeny

		m.interaction.confirmResponse = nil

		return true, nil
	}

	// Priority 0b: Dismiss pending ask
	if m.interaction.askResponse != nil {
		if m.picker != nil {
			m.picker = nil
		}

		m.interaction.askResponse <- ""

		m.clearAskState()

		return true, nil
	}

	// Priority 1: Copy selection if exists
	if m.hasSelection() {
		cmd := m.copySelectionToClipboard()
		m.clearSelection()
		m.updateViewportContent()

		return true, cmd
	}

	// Priority 2: Clear textarea if has content
	if m.textarea.Value() != "" {
		m.textarea.Reset()
		m.textarea.SetHeight(1)
		m.adjustTextareaHeight()

		m.exit.ctrlCPending = false

		return true, nil
	}

	// Priority 3: Exit confirmation
	if m.exit.ctrlCPending && time.Since(m.exit.ctrlCTime) < 2*time.Second {
		return true, tea.Quit
	}

	// Show warning in placeholder and schedule auto-clear
	m.exit.ctrlCPending = true
	m.exit.ctrlCTime = time.Now()
	m.textarea.Placeholder = "Press Ctrl+C again to exit..."

	clearCmd := tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ctrlCTimeoutMsg{}
	})

	return true, clearCmd
}

// handleEnter processes Enter key to submit input.
func (m *Model) handleEnter() (bool, tea.Cmd) {
	text := m.textarea.Value()
	if text == "" {
		return true, nil
	}

	// Intercept ask response
	if m.interaction.askResponse != nil {
		response := ui.ResolveAskResponse(text, m.interaction.askOptions, m.interaction.askMulti)
		m.interaction.askResponse <- response

		// Show user's answer in chat
		m.messages = append(m.messages, message.Message{
			Role:  roles.User,
			Parts: []part.Part{{Type: part.Content, Text: response}},
		})

		m.clearAskState()
		m.textarea.Reset()
		m.textarea.SetHeight(1)
		m.adjustTextareaHeight()
		m.updateViewportContent()
		m.viewport.GotoBottom()

		return true, nil
	}

	m.clearFlash()

	m.hist.Add(text)
	m.hist.Reset()

	// Reset textarea
	m.textarea.Reset()
	m.textarea.SetHeight(1)
	m.adjustTextareaHeight()

	// Queue as pending - backend will confirm via UserMessageProcessed
	m.pendingMessages = append(m.pendingMessages, text)

	// Send to backend
	go func() {
		m.input <- ui.UserInput{Text: text}
	}()

	return true, nil
}

// handleHistoryUp navigates to older history entries.
func (m *Model) handleHistoryUp() (bool, tea.Cmd) {
	if text, ok := m.hist.Up(m.textarea.Value()); ok {
		m.textarea.Reset()
		m.textarea.SetValue(text)
		m.adjustTextareaHeight()
	}

	return true, nil
}

// handleHistoryDown navigates to newer history entries.
func (m *Model) handleHistoryDown() (bool, tea.Cmd) {
	if text, ok := m.hist.Down(); ok {
		m.textarea.Reset()
		m.textarea.SetValue(text)
		m.adjustTextareaHeight()
	}

	return true, nil
}

// handleMouseMsg processes mouse events for scrolling and selection.
// Returns (handled, cmd) where handled indicates if the event was consumed.
func (m *Model) handleMouseMsg(msg tea.MouseMsg) (bool, tea.Cmd) {
	switch msg.(type) {
	case tea.MouseWheelMsg:
		var cmd tea.Cmd

		m.viewport, cmd = m.viewport.Update(msg)
		m.scrollLocked = !m.viewport.AtBottom()

		return true, cmd
	}

	mouse := msg.Mouse()

	switch msg.(type) {
	case tea.MouseClickMsg:
		if mouse.Button == tea.MouseLeft {
			viewportY := mouse.Y - m.selection.viewportTop
			if viewportY < 0 || viewportY >= m.viewport.Height() {
				return false, nil
			}

			m.clearSelection()
			m.startSelection(mouse.X, viewportY+m.viewport.YOffset())
			m.updateViewportContent()

			return true, nil
		}

	case tea.MouseMotionMsg:
		if m.selection.active {
			viewportY := mouse.Y - m.selection.viewportTop

			viewportY = max(0, min(viewportY, m.viewport.Height()-1))
			m.extendSelection(mouse.X, viewportY+m.viewport.YOffset())
			m.updateViewportContent()

			return true, nil
		}

	case tea.MouseReleaseMsg:
		if mouse.Button == tea.MouseLeft && m.selection.active {
			m.endSelection()

			var cmd tea.Cmd

			if m.hasSelection() {
				cmd = m.copySelectionToClipboard()
			}

			m.updateViewportContent()

			return true, cmd
		}
	}

	return false, nil
}

// handleMessageAdded processes a complete message being added.
func (m *Model) handleMessageAdded(msg ui.MessageAdded) {
	m.messages = append(m.messages, msg.Message)
	m.updateViewportContent()

	if !m.scrollLocked {
		m.viewport.GotoBottom()
	}
}

// handleMessageStarted processes the start of a new streaming message.
func (m *Model) handleMessageStarted(msg ui.MessageStarted) {
	m.currentMessage = &message.Message{
		ID:    msg.MessageID,
		Role:  roles.Assistant,
		Parts: []part.Part{},
	}
	m.spinnerMsg = m.spintext.Random()
	m.updateViewportContent()
}

// handleMessagePartAdded processes a new part added to the current message.
func (m *Model) handleMessagePartAdded(msg ui.MessagePartAdded) {
	if m.currentMessage == nil {
		return
	}

	m.currentMessage.Parts = append(m.currentMessage.Parts, msg.Part)
	m.updateViewportContent()

	if !m.scrollLocked {
		m.viewport.GotoBottom()
	}
}

// handleMessagePartUpdated processes an update to an existing message part.
func (m *Model) handleMessagePartUpdated(msg ui.MessagePartUpdated) {
	if m.currentMessage == nil || msg.PartIndex >= len(m.currentMessage.Parts) {
		return
	}

	m.currentMessage.Parts[msg.PartIndex] = msg.Part
	m.updateViewportContent()

	if !m.scrollLocked {
		m.viewport.GotoBottom()
	}
}

// handleMessageFinalized processes the completion of a streaming message.
func (m *Model) handleMessageFinalized(msg ui.MessageFinalized) {
	if m.currentMessage == nil {
		return
	}

	m.messages = append(m.messages, msg.Message)
	m.currentMessage = nil
	m.spinnerMsg = "" // Clear streaming spinner — standalone spinners (compaction) manage their own lifecycle
	m.toolOutputLine = ""
	m.updateViewportContent()
}

// handleUserMessagesProcessed processes backend acknowledgment of user inputs.
func (m *Model) handleUserMessagesProcessed(msg ui.UserMessagesProcessed) {
	// Remove all processed messages from pending queue
	for _, text := range msg.Texts {
		if idx := slices.Index(m.pendingMessages, text); idx >= 0 {
			m.pendingMessages = slices.Delete(m.pendingMessages, idx, idx+1)
		}
	}

	// Start streaming state for assistant response
	m.scrollLocked = false
	m.state = StateStreaming
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

// handlePagerKey routes key events to the pager overlay.
func (m *Model) handlePagerKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.pager.HandleKey(msg) {
		m.pager = nil
	}

	return true, nil
}

// handlePickerKey routes key events to the picker overlay.
func (m *Model) handlePickerKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	selected, dismissed := m.picker.HandleKey(msg)

	if dismissed {
		m.picker = nil

		// If confirm is active, dismissing picker denies the command
		if m.interaction.confirmResponse != nil {
			m.interaction.confirmResponse <- ui.ConfirmDeny

			m.interaction.confirmResponse = nil

			return true, nil
		}

		// If ask is active, dismissing picker dismisses the question
		if m.interaction.askResponse != nil {
			m.interaction.askResponse <- ""

			m.clearAskState()
		}

		return true, nil
	}

	if selected != nil {
		m.picker = nil

		// If confirm is active, map action to response
		if m.interaction.confirmResponse != nil {
			switch selected.Action.(type) {
			case ui.ConfirmOnce:
				m.interaction.confirmResponse <- ui.ConfirmAllow
			case ui.ConfirmSession:
				m.interaction.confirmResponse <- ui.ConfirmAllowSession
			case ui.ConfirmPatternProject:
				m.interaction.confirmResponse <- ui.ConfirmAllowPatternProject
			case ui.ConfirmPatternGlobal:
				m.interaction.confirmResponse <- ui.ConfirmAllowPatternGlobal
			case ui.ConfirmReject:
				m.interaction.confirmResponse <- ui.ConfirmDeny
			default:
				m.interaction.confirmResponse <- ui.ConfirmDeny
			}

			m.interaction.confirmResponse = nil

			return true, nil
		}

		// If ask is active, selection resolves the question
		if m.interaction.askResponse != nil {
			m.interaction.askResponse <- selected.Label

			m.clearAskState()

			return true, nil
		}

		if selected.Action != nil {
			go func() {
				m.actions <- ui.UserAction{Action: selected.Action}
			}()
		}

		return true, nil
	}

	return true, nil
}

// handleSyntheticInjected appends a synthetic injection as a persistent viewport message.
func (m *Model) handleSyntheticInjected(msg ui.SyntheticInjected) {
	text := msg.Content
	if msg.Header != "" {
		text = msg.Header + "\n" + msg.Content
	}

	m.messages = append(m.messages, message.Message{
		Role:  roles.System,
		Parts: []part.Part{{Type: part.Content, Text: text}},
	})
	m.updateViewportContent()

	if !m.scrollLocked {
		m.viewport.GotoBottom()
	}
}

// handleToolConfirm processes a ToolConfirmRequired event from the tool policy.
func (m *Model) handleToolConfirm(msg ui.ToolConfirmRequired) {
	m.interaction.confirmResponse = msg.Response

	title := fmt.Sprintf("Confirm %s: %s", msg.ToolName, msg.Detail)
	if msg.Description != "" {
		title += "\n" + msg.Description
	}

	// Use ConfirmPager with diff preview when available, Picker otherwise.
	if msg.DiffPreview != "" {
		highlighted := highlightDiff(msg.DiffPreview)

		m.confirmPager = NewConfirmPager(title, highlighted, msg.Pattern, msg.Response, m.width, m.viewport.Height())

		return
	}

	items := []ui.PickerItem{
		{
			Label:       "Allow",
			Description: "Run this tool call once",
			Action:      ui.ConfirmOnce{},
		},
		{
			Label:       fmt.Sprintf("Allow \"%s\" (session)", msg.Pattern),
			Description: "Approve for this session only",
			Action:      ui.ConfirmSession{},
		},
		{
			Label:       fmt.Sprintf("Allow \"%s\" (project)", msg.Pattern),
			Description: "Persist to project approval rules",
			Action:      ui.ConfirmPatternProject{},
		},
		{
			Label:       fmt.Sprintf("Allow \"%s\" (global)", msg.Pattern),
			Description: "Persist to global approval rules",
			Action:      ui.ConfirmPatternGlobal{},
		},
		{
			Label:       "Deny",
			Description: "Block this tool call",
			Action:      ui.ConfirmReject{},
		},
	}

	m.picker = NewPicker(title, items, m.width, m.viewport.Height())
}

// handleConfirmPagerKey processes key events for the ConfirmPager overlay.
func (m *Model) handleConfirmPagerKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	action, _ := m.confirmPager.HandleKey(msg)
	if action != nil {
		m.interaction.confirmResponse <- *action

		m.interaction.confirmResponse = nil
		m.confirmPager = nil
	}

	return true, nil
}

// handleAskRequired processes an AskRequired event from the Ask tool.
func (m *Model) handleAskRequired(msg ui.AskRequired) {
	m.interaction.askResponse = msg.Response
	m.interaction.askOptions = msg.Options
	m.interaction.askMulti = msg.MultiSelect

	if len(msg.Options) > 0 && !msg.MultiSelect {
		// Single-select: use picker overlay
		items := make([]ui.PickerItem, len(msg.Options))
		for i, o := range msg.Options {
			items[i] = ui.PickerItem{
				Label:       o.Label,
				Description: o.Description,
			}
		}

		m.picker = NewPicker(msg.Question, items, m.width, m.viewport.Height())

		return
	}

	// Multi-select or free-form: show question in chat, accept textarea input
	var questionText string

	if len(msg.Options) > 0 {
		lines := []string{msg.Question}
		for i, o := range msg.Options {
			line := fmt.Sprintf("  %d. %s", i+1, o.Label)
			if o.Description != "" {
				line += " — " + o.Description
			}

			lines = append(lines, line)
		}

		lines = append(lines, "\nEnter number(s) or text:")
		questionText = strings.Join(lines, "\n")
	} else {
		questionText = msg.Question
	}

	m.messages = append(m.messages, message.Message{
		Role:  roles.System,
		Parts: []part.Part{{Type: part.Content, Text: questionText}},
	})

	m.textarea.Placeholder = "answer..."
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

// clearAskState resets all ask-related fields.
func (m *Model) clearAskState() {
	m.interaction.askResponse = nil
	m.interaction.askOptions = nil
	m.interaction.askMulti = false

	m.textarea.Placeholder = "Type your message..."
}

// handleCommandResult shows slash command output as a transient flash message.
func (m *Model) handleCommandResult(msg ui.CommandResult) tea.Cmd {
	if msg.Clear {
		m.messages = message.Messages{}
		m.currentMessage = nil
		m.clearFlash()
		m.updateViewportContent()

		return nil
	}

	if msg.Command != "" {
		m.messages = append(m.messages, message.Message{
			Role:  roles.User,
			Parts: []part.Part{{Type: part.Content, Text: msg.Command}},
		})
	}

	var cmd tea.Cmd

	if msg.Error != nil {
		cmd = m.setFlash("Error: "+msg.Error.Error(), ui.LevelError)
	} else if msg.Message != "" {
		if !msg.Inline && strings.Contains(msg.Message, "\n") {
			m.pager = NewPager("Command output", msg.Message, m.width, m.viewport.Height())
		} else {
			cmd = m.setFlash(msg.Message, msg.Level)
		}
	}

	m.updateViewportContent()
	m.viewport.GotoBottom()

	return cmd
}

// lastCompletedToolCall returns the most recent completed (non-error) tool call
// from the current streaming message and finalized messages.
func (m *Model) lastCompletedToolCall() *call.Call {
	// Check current streaming message first (most recent activity).
	if m.currentMessage != nil {
		for i := len(m.currentMessage.Parts) - 1; i >= 0; i-- {
			p := m.currentMessage.Parts[i]
			if p.IsTool() && p.Call != nil && p.Call.State == call.Complete {
				return p.Call
			}
		}
	}

	// Then finalized messages in reverse.
	for i := len(m.messages) - 1; i >= 0; i-- {
		for j := len(m.messages[i].Parts) - 1; j >= 0; j-- {
			p := m.messages[i].Parts[j]
			if p.IsTool() && p.Call != nil && p.Call.State == call.Complete {
				return p.Call
			}
		}
	}

	return nil
}
