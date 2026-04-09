package tui

import (
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/ui"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles all messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		debug.Log("[TUI] key: %q (code=%d, state=%d)", msg.String(), msg.Code, m.state)

		if handled, cmd := m.handleKeyMsg(msg); handled {
			return m, cmd
		}

	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)
		if m.state == StateStreaming || m.spinnerMsg != "" {
			m.updateViewportContent()
		}

		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(m.width - textareaPadding)
		m.recalculateLayout()
		m.updateViewportContent()

	case tea.MouseMsg:
		if handled, cmd := m.handleMouseMsg(msg); handled {
			return m, cmd
		}

	case ui.MessageAdded:
		m.handleMessageAdded(msg)

	case ui.MessageStarted:
		m.handleMessageStarted(msg)

	case ui.MessagePartAdded:
		m.handleMessagePartAdded(msg)

	case ui.MessagePartUpdated:
		m.handleMessagePartUpdated(msg)

	case ui.MessageFinalized:
		m.handleMessageFinalized(msg)

	case ui.SessionRestored:
		m.messages = msg.Messages
		m.updateViewportContent()
		m.viewport.GotoBottom()

		return m, nil

	case ui.StatusChanged:
		m.status = msg.Status

		return m, nil

	case ui.DisplayHintsChanged:
		m.hints = msg.Hints

		return m, nil

	case ui.CommandResult:
		cmd := m.handleCommandResult(msg)

		return m, cmd

	case ui.TodoEditRequested:
		return m, openEditor(msg.Content)

	case editorFinishedMsg:
		if msg.err != nil {
			return m, m.setFlash("Editor error: "+msg.err.Error(), ui.LevelError)
		}

		go func() {
			m.actions <- ui.UserAction{Action: ui.TodoEdited{Text: msg.content}}
		}()

		return m, nil

	case ui.PickerOpen:
		m.picker = NewPicker(msg.Title, msg.Items, m.width, m.viewport.Height())

		return m, nil

	case ui.AskRequired:
		m.handleAskRequired(msg)

		return m, nil

	case ui.ToolConfirmRequired:
		m.handleToolConfirm(msg)

		return m, nil

	case ui.AssistantDone:
		cmds = append(cmds, m.finishStreaming())

	case ui.WaitingForInput:
		cmds = append(cmds, m.finishStreaming())

	case ui.UserMessagesProcessed:
		m.handleUserMessagesProcessed(msg)

	case ui.SlashCommandHandled:
		// Remove from pending but don't switch to streaming state
		for i, pending := range m.pendingMessages {
			if pending == msg.Text {
				m.pendingMessages = append(m.pendingMessages[:i], m.pendingMessages[i+1:]...)

				break
			}
		}

		return m, nil

	case ui.SyntheticInjected:
		m.handleSyntheticInjected(msg)

		return m, nil

	case ui.SpinnerMessage:
		m.spinnerMsg = msg.Text
		m.toolOutputLine = "" // New spinner context clears stale tool output
		m.updateViewportContent()

		return m, nil

	case ui.ToolOutputDelta:
		m.toolOutputLine = msg.Line
		m.updateViewportContent()

		return m, nil

	case ui.Flush:
		close(msg.Done)

		return m, nil

	case ctrlCTimeoutMsg:
		// Clear the exit warning if still pending
		if m.exit.ctrlCPending {
			m.exit.ctrlCPending = false
			m.textarea.Placeholder = "Type your message..."
		}

	case clearFlashMsg:
		if msg.gen == m.flash.gen {
			m.clearFlash()
		}
	}

	// Always update textarea - user can type during streaming
	var cmd tea.Cmd

	m.textarea, cmd = m.textarea.Update(msg)
	m.adjustTextareaHeight()

	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}
