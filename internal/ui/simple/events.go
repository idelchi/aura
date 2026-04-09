package simple

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/godyl/pkg/path/file"
)

func (s *Simple) processEvent(event ui.Event) {
	switch e := event.(type) {
	case ui.MessageAdded:
		s.renderCh <- func() { s.handleMessageAdded(e) }
	case ui.MessageStarted:
		s.renderCh <- func() { s.handleMessageStarted() }
	case ui.MessagePartAdded:
		s.renderCh <- func() { s.handleMessagePartAdded(e) }
	case ui.MessagePartUpdated:
		s.renderCh <- func() { s.handleMessagePartUpdated(e) }
	case ui.MessageFinalized:
		s.renderCh <- func() { s.handleMessageFinalized(e) }
	case ui.StatusChanged:
		s.renderCh <- func() { s.status = e.Status }
	case ui.DisplayHintsChanged:
		s.renderCh <- func() { s.hints = e.Hints }
	case ui.WaitingForInput:
		s.renderCh <- func() { s.handleWaitingForInput() }
	case ui.CommandResult:
		s.renderCh <- func() { s.handleCommandResult(e) }
	case ui.AssistantDone:
		s.renderCh <- func() { s.handleAssistantDone(e) }
	case ui.SyntheticInjected:
		s.renderCh <- func() {
			s.spinner.Stop()
			ui.SyntheticStyle.Printf("\n%s\n%s\n", e.Header, e.Content)
		}
	case ui.ToolConfirmRequired:
		s.renderCh <- func() { s.handleToolConfirm(e) }
	case ui.AskRequired:
		s.renderCh <- func() { s.handleAskRequired(e) }
	case ui.PickerOpen:
		s.renderCh <- func() {
			s.rl.Clean()
			fmt.Print(e.Display())
			s.rl.Refresh()
		}
	case ui.TodoEditRequested:
		s.renderCh <- func() { s.handleTodoEditRequested(e) }
	case ui.SpinnerMessage:
		s.renderCh <- func() { s.handleSpinnerMessage(e) }
	case ui.ToolOutputDelta:
		s.renderCh <- func() { s.handleToolOutputDelta(e) }
	case ui.UserMessagesProcessed:
		// No-op for simple UI
	case ui.SessionRestored:
		// No-op for simple UI
	case ui.SlashCommandHandled:
		// No-op for simple UI
	case ui.Flush:
		close(e.Done) // No state access — stays direct
	}
}

func (s *Simple) handleMessageAdded(e ui.MessageAdded) {
	if e.Message.IsBookmark() {
		fmt.Fprintf(os.Stderr, "\n--- %s ---\n", strings.TrimSpace(e.Message.Content))

		return
	}

	if e.Message.IsDisplayOnly() {
		ui.SyntheticStyle.Printf("\n%s\n", strings.TrimSpace(e.Message.Content))

		return
	}

	if e.Message.Role == roles.User {
		ui.UserStyle.Printf("%s%s\n", s.status.UserPrompt(), strings.TrimSpace(e.Message.Content))
	}
}

func (s *Simple) handleMessageStarted() {
	s.state = StateStreaming
	s.tracker.Reset()
	s.spinner.Update(s.spintext.Random())
	s.spinner.Start()
}

func (s *Simple) handleMessagePartAdded(e ui.MessagePartAdded) {
	s.spinner.Stop()
	ui.RenderPartAdded(s.tracker.Added(e.Part), e.Part, s.hints.Verbose, s.status.AssistantPrompt())
}

func (s *Simple) handleMessagePartUpdated(e ui.MessagePartUpdated) {
	s.spinner.Stop()
	ui.RenderPartUpdated(s.tracker.Updated(e.Part), e.Part, s.hints.Verbose, s.status.AssistantPrompt())
}

func (s *Simple) handleMessageFinalized(e ui.MessageFinalized) {
	s.spinner.Stop()

	if e.Message.Error != nil {
		ui.ErrorStyle.Printf("\nError: %v\n", e.Message.Error)

		s.lastErrorShown = true
	} else {
		fmt.Println()

		s.lastErrorShown = false
	}

	s.state = StateIdle
	s.tracker.Reset()
	s.rl.Refresh()
}

func (s *Simple) handleWaitingForInput() {
	s.spinner.Stop()

	s.state = StateIdle
	s.rl.Refresh()
}

func (s *Simple) handleCommandResult(e ui.CommandResult) {
	if e.Clear {
		fmt.Print("\033[2J\033[H")
		s.rl.Refresh()

		return
	}

	s.rl.Clean()

	if e.Command != "" {
		ui.UserStyle.Printf("%s%s\n", s.status.UserPrompt(), e.Command)
	}

	if e.Error != nil {
		ui.ErrorStyle.Printf("Error: %v\n", e.Error)
	} else if e.Message != "" {
		fmt.Println(strings.TrimSpace(e.Message))
	}

	s.rl.Refresh()
}

func (s *Simple) handleAssistantDone(e ui.AssistantDone) {
	s.spinner.Stop()

	s.state = StateIdle

	if e.Cancelled {
		fmt.Println("\n(cancelled)")
	} else if e.Error != nil && !s.lastErrorShown {
		ui.ErrorStyle.Printf("\nError: %v\n", e.Error)
	}

	s.lastErrorShown = false
	s.rl.Refresh()
}

func (s *Simple) handleTodoEditRequested(e ui.TodoEditRequested) {
	tmp, err := file.CreateRandomInDir("", "aura-todo-*.md")
	if err != nil {
		ui.ErrorStyle.Printf("Error: %v\n", err)

		return
	}

	defer tmp.Remove()

	f, err := tmp.OpenForWriting()
	if err != nil {
		ui.ErrorStyle.Printf("Error: %v\n", err)

		return
	}

	if _, err := f.WriteString(e.Content); err != nil {
		f.Close()
		ui.ErrorStyle.Printf("Error: %v\n", err)

		return
	}

	f.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, tmp.Path())

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		ui.ErrorStyle.Printf("Editor error: %v\n", err)

		return
	}

	data, err := tmp.Read()
	if err != nil {
		ui.ErrorStyle.Printf("Error reading file: %v\n", err)

		return
	}

	go func() {
		s.actions <- ui.UserAction{Action: ui.TodoEdited{Text: string(data)}}
	}()
}

func (s *Simple) handleToolConfirm(e ui.ToolConfirmRequired) {
	s.spinner.Stop()
	s.rl.Clean()

	fmt.Println()

	if e.DiffPreview != "" {
		fmt.Println(highlightDiffSimple(e.DiffPreview))
		fmt.Println()
	}

	ui.AssistantStyle.Printf("? Confirm %s: %s\n", e.ToolName, e.Detail)

	if e.Description != "" {
		ui.ToolStyle.Printf("  %s\n", e.Description)
	}

	ui.ToolStyle.Println("  1. Allow")
	ui.ToolStyle.Printf("  2. Allow \"%s\" (session)\n", e.Pattern)
	ui.ToolStyle.Printf("  3. Allow \"%s\" (project)\n", e.Pattern)
	ui.ToolStyle.Printf("  4. Allow \"%s\" (global)\n", e.Pattern)
	ui.ToolStyle.Println("  5. Deny")
	fmt.Println()

	s.pendingConfirm = &e

	s.rl.SetPrompt(successStyle.Sprint("confirm> "))
	s.rl.Refresh()
}

// highlightDiffSimple applies Chroma ANSI highlighting to unified diff content.
func highlightDiffSimple(content string) string {
	lexer := lexers.Get("diff")
	if lexer == nil {
		return content
	}

	lexer = chroma.Coalesce(lexer)

	iter, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content
	}

	formatter := formatters.Get("terminal256")
	style := styles.Get("monokai")

	var buf strings.Builder
	if err := formatter.Format(&buf, style, iter); err != nil {
		return content
	}

	return buf.String()
}

func (s *Simple) handleAskRequired(e ui.AskRequired) {
	s.spinner.Stop()
	s.rl.Clean()

	// Print question
	fmt.Println()
	ui.AssistantStyle.Printf("? %s\n", e.Question)

	// Print numbered options
	for i, o := range e.Options {
		line := fmt.Sprintf("  %d. %s", i+1, o.Label)
		if o.Description != "" {
			line += " — " + o.Description
		}

		ui.ToolStyle.Println(line)
	}

	if len(e.Options) > 0 && e.MultiSelect {
		ui.ToolStyle.Println("\n  Enter comma-separated numbers or text:")
	}

	fmt.Println()

	// Store pending ask and change prompt
	s.pendingAsk = &e

	s.rl.SetPrompt(successStyle.Sprint("answer> "))
	s.rl.Refresh()
}

func (s *Simple) handleSpinnerMessage(e ui.SpinnerMessage) {
	if s.state == StateStreaming {
		return
	}

	if e.Text == "" {
		s.spinner.Stop()

		return
	}

	s.spinner.Update(e.Text)
}

func (s *Simple) handleToolOutputDelta(e ui.ToolOutputDelta) {
	if s.state == StateStreaming {
		return
	}

	s.spinner.Update(fmt.Sprintf("%s: %s", e.ToolName, e.Line))
}
