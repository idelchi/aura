package tui

import (
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/ui"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles all messages and updates the model.
// The activity observes every assistant event before it is handled, and follows the
// model's dialog state after every message, so no handler has to remember either.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	now := m.clock()

	if event, ok := msg.(ui.Event); ok {
		m.activity.Observe(event, now)
	}

	cmd := m.update(msg)

	m.activity.AwaitUser(m.awaitingUser(), now)

	return m, cmd
}

// awaitingUser reports which dialog, if any, blocks the assistant on the user.
func (m Model) awaitingUser() ui.Awaiting {
	switch {
	case m.interaction.confirmResponse != nil:
		return ui.AwaitingConfirmation
	case m.interaction.askResponse != nil:
		return ui.AwaitingAnswer
	default:
		return ui.AwaitingNothing
	}
}

// update applies msg to the model.
func (m *Model) update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		debug.Log("[TUI] key: %q (code=%d, state=%d)", msg.String(), msg.Code, m.state)

		if handled, cmd := m.handleKeyMsg(msg); handled {
			return cmd
		}

	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)
		// Running tool headers in the history animate with the spinner.
		if m.state == StateStreaming {
			m.updateViewportContent()
		}

		return cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(m.width - textareaPadding)
		m.recalculateLayout()
		m.updateViewportContent()

	case tea.MouseMsg:
		if handled, cmd := m.handleMouseMsg(msg); handled {
			return cmd
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

		return nil

	case ui.StatusChanged:
		m.status = msg.Status

		return nil

	case ui.DisplayHintsChanged:
		m.hints = msg.Hints
		m.updateViewportContent()

		return nil

	case ui.CommandResult:
		return m.handleCommandResult(msg)

	case ui.TodoEditRequested:
		return openEditor(msg.Content)

	case editorFinishedMsg:
		if msg.err != nil {
			return m.setFlash("Editor error: "+msg.err.Error(), ui.LevelError)
		}

		go func() {
			m.actions <- ui.UserAction{Action: ui.TodoEdited{Text: msg.content}}
		}()

		return nil

	case ui.PickerOpen:
		m.picker = NewPicker(msg.Title, msg.Items, m.width, m.viewport.Height())

		return nil

	case ui.AskRequired:
		m.handleAskRequired(msg)

		return nil

	case ui.ToolConfirmRequired:
		m.handleToolConfirm(msg)

		return nil

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

		return nil

	case ui.SyntheticInjected:
		m.handleSyntheticInjected(msg)

		return nil

	case ui.SpinnerMessage:
		m.toolOutputLine = "" // New spinner context clears stale tool output
		m.updateViewportContent()

		return nil

	case ui.ToolOutputDelta:
		m.toolOutputLine = msg.Line
		m.updateViewportContent()

		return nil

	case ui.Flush:
		close(msg.Done)

		return nil

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

	return tea.Batch(cmds...)
}
