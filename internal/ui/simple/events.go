package simple

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/godyl/pkg/path/file"
)

// handleEvent renders one event on Run's goroutine, in arrival order. A flush is
// therefore released only after every earlier event has been rendered.
// The activity observes every event; the prompt is redrawn afterwards if it changed.
func (s *Simple) handleEvent(event ui.Event) {
	s.activity.Observe(event, s.clock())

	switch e := event.(type) {
	case ui.MessageAdded:
		s.handleMessageAdded(e)
	case ui.MessageStarted:
		s.tracker.Reset()
	case ui.MessagePartAdded:
		s.handleMessagePartAdded(e)
	case ui.MessagePartUpdated:
		s.handleMessagePartUpdated(e)
	case ui.MessageFinalized:
		s.handleMessageFinalized(e)
	case ui.StatusChanged:
		s.status = e.Status
	case ui.DisplayHintsChanged:
		s.hints = e.Hints
	case ui.WaitingForInput:
		// No-op: the activity follows the turn's events
	case ui.CommandResult:
		s.handleCommandResult(e)
	case ui.AssistantDone:
		s.handleAssistantDone(e)
	case ui.SyntheticInjected:
		s.clearPrompt()
		ui.SyntheticStyle.Fprintf(s.out, "\n%s\n%s\n", e.Header, e.Content)
		s.printed(ui.LineStart)
	case ui.ToolConfirmRequired:
		s.handleToolConfirm(e)
	case ui.AskRequired:
		s.handleAskRequired(e)
	case ui.PickerOpen:
		s.clearPrompt()
		s.endLine()
		fmt.Fprint(s.out, e.Display())
		s.printed(ui.LineStart)
	case ui.TodoEditRequested:
		s.handleTodoEditRequested(e)
	case ui.SpinnerMessage:
		// No-op: shown by the activity prompt
	case ui.ToolOutputDelta:
		// No-op: counts as progress for the activity prompt
	case ui.UserMessagesProcessed:
		// No-op for simple UI
	case ui.SessionRestored:
		// No-op for simple UI
	case ui.SlashCommandHandled:
		// No-op for simple UI
	case ui.Flush:
		close(e.Done)
	}

	s.redraw()
}

func (s *Simple) handleMessageAdded(e ui.MessageAdded) {
	switch {
	case e.Message.IsBookmark():
		s.clearPrompt()
		fmt.Fprintf(s.out, "\n--- %s ---\n", strings.TrimSpace(e.Message.Content))
	case e.Message.IsDisplayOnly():
		s.clearPrompt()
		ui.SyntheticStyle.Fprintf(s.out, "\n%s\n", strings.TrimSpace(e.Message.Content))
	case e.Message.Role == roles.User:
		s.clearPrompt()
		s.endLine()
		ui.UserStyle.Fprintf(s.out, "%s%s\n", s.status.UserPrompt(), strings.TrimSpace(e.Message.Content))
	default:
		return
	}

	s.printed(ui.LineStart)
}

func (s *Simple) handleMessagePartAdded(e ui.MessagePartAdded) {
	d := s.tracker.Added(e.Part)

	if e.Part.IsTool() {
		// Registered only — its header prints once it runs. End an open streamed line
		// now, so the header does not continue the text.
		s.endLine()

		return
	}

	s.renderPart(d, e.Part, ui.RenderPartAdded)
}

func (s *Simple) handleMessagePartUpdated(e ui.MessagePartUpdated) {
	s.renderPart(s.tracker.Updated(e.Part), e.Part, ui.RenderPartUpdated)
}

// renderPart prints a streamed part. The prompt is removed first unless the part prints
// nothing (hidden thinking within a thinking section), which avoids redrawing it per delta.
func (s *Simple) renderPart(d ui.Delta, p part.Part, render func(io.Writer, ui.Delta, part.Part, bool, string) ui.Line) {
	if !(p.IsThinking() && !s.hints.Verbose && !d.SectionBreak) {
		s.clearPrompt()
	}

	s.printed(render(s.out, d, p, s.hints.Verbose, s.status.AssistantPrompt()))
}

func (s *Simple) handleMessageFinalized(e ui.MessageFinalized) {
	s.clearPrompt()

	if e.Message.Error != nil {
		ui.ErrorStyle.Fprintf(s.out, "\nError: %v\n", e.Message.Error)

		s.lastErrorShown = true
	} else {
		fmt.Fprintln(s.out)

		s.lastErrorShown = false
	}

	s.printed(ui.LineStart)
	s.tracker.Reset()
}

func (s *Simple) handleCommandResult(e ui.CommandResult) {
	s.clearPrompt()

	if e.Clear {
		s.endLine()
		fmt.Fprint(s.out, "\033[2J\033[H")
		s.printed(ui.LineStart)

		return
	}

	if e.Command == "" && e.Error == nil && e.Message == "" {
		return
	}

	s.endLine()

	if e.Command != "" {
		ui.UserStyle.Fprintf(s.out, "%s%s\n", s.status.UserPrompt(), e.Command)
	}

	if e.Error != nil {
		ui.ErrorStyle.Fprintf(s.out, "Error: %v\n", e.Error)
	} else if e.Message != "" {
		fmt.Fprintln(s.out, strings.TrimSpace(e.Message))
	}

	s.printed(ui.LineStart)
}

func (s *Simple) handleAssistantDone(e ui.AssistantDone) {
	switch {
	case e.Cancelled:
		s.clearPrompt()
		fmt.Fprintln(s.out, "\n(cancelled)")
		s.printed(ui.LineStart)
	case e.Error != nil && !s.lastErrorShown:
		s.clearPrompt()
		ui.ErrorStyle.Fprintf(s.out, "\nError: %v\n", e.Error)
		s.printed(ui.LineStart)
	}

	s.lastErrorShown = false
}

func (s *Simple) handleTodoEditRequested(e ui.TodoEditRequested) {
	s.clearPrompt()
	s.endLine()

	// The editor takes over the terminal; whatever it leaves, continue on a fresh line.
	defer s.printed(ui.LineStart)

	tmp, err := file.CreateRandomInDir("", "aura-todo-*.md")
	if err != nil {
		ui.ErrorStyle.Fprintf(s.out, "Error: %v\n", err)

		return
	}

	defer tmp.Remove()

	f, err := tmp.OpenForWriting()
	if err != nil {
		ui.ErrorStyle.Fprintf(s.out, "Error: %v\n", err)

		return
	}

	if _, err := f.WriteString(e.Content); err != nil {
		f.Close()
		ui.ErrorStyle.Fprintf(s.out, "Error: %v\n", err)

		return
	}

	f.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, tmp.Path())

	cmd.Stdin = os.Stdin
	cmd.Stdout = s.terminal
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		ui.ErrorStyle.Fprintf(s.out, "Editor error: %v\n", err)

		return
	}

	data, err := tmp.Read()
	if err != nil {
		ui.ErrorStyle.Fprintf(s.out, "Error reading file: %v\n", err)

		return
	}

	go func() {
		s.actions <- ui.UserAction{Action: ui.TodoEdited{Text: string(data)}}
	}()
}

func (s *Simple) handleToolConfirm(e ui.ToolConfirmRequired) {
	s.clearPrompt()

	fmt.Fprintln(s.out)

	if e.DiffPreview != "" {
		fmt.Fprintln(s.out, highlightDiffSimple(e.DiffPreview))
		fmt.Fprintln(s.out)
	}

	ui.AssistantStyle.Fprintf(s.out, "? Confirm %s: %s\n", e.ToolName, e.Detail)

	if e.Description != "" {
		ui.ToolStyle.Fprintf(s.out, "  %s\n", e.Description)
	}

	ui.ToolStyle.Fprintln(s.out, "  1. Allow")
	ui.ToolStyle.Fprintf(s.out, "  2. Allow \"%s\" (session)\n", e.Pattern)
	ui.ToolStyle.Fprintf(s.out, "  3. Allow \"%s\" (project)\n", e.Pattern)
	ui.ToolStyle.Fprintf(s.out, "  4. Allow \"%s\" (global)\n", e.Pattern)
	ui.ToolStyle.Fprintln(s.out, "  5. Deny")
	fmt.Fprintln(s.out)

	s.printed(ui.LineStart)

	// The prompt switches to "confirm> " on the redraw that follows every event.
	s.pendingConfirm = &e
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
	s.clearPrompt()

	// Print question
	fmt.Fprintln(s.out)
	ui.AssistantStyle.Fprintf(s.out, "? %s\n", e.Question)

	// Print numbered options
	for i, o := range e.Options {
		line := fmt.Sprintf("  %d. %s", i+1, o.Label)
		if o.Description != "" {
			line += " — " + o.Description
		}

		ui.ToolStyle.Fprintln(s.out, line)
	}

	if len(e.Options) > 0 && e.MultiSelect {
		ui.ToolStyle.Fprintln(s.out, "\n  Enter comma-separated numbers or text:")
	}

	fmt.Fprintln(s.out)

	s.printed(ui.LineStart)

	// The prompt switches to "answer> " on the redraw that follows every event.
	s.pendingAsk = &e
}
