package tui

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"

	"charm.land/bubbles/v2/spinner"
)

func renderTestModel(verbose bool) Model {
	return Model{
		width:   60,
		hints:   ui.DisplayHints{Verbose: verbose},
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

func thinkingPart(text string) part.Part { return part.Part{Type: part.Thinking, Text: text} }

func contentPart(text string) part.Part { return part.Part{Type: part.Content, Text: text} }

func toolPart(c *call.Call) part.Part { return part.Part{Type: part.Tool, Call: c} }

func bashCall(id, command string) *call.Call {
	c := &call.Call{ID: id, Name: "Bash"}
	c.SetArgs(map[string]any{"command": command})

	return c
}

func completedBash(id, command, result string) *call.Call {
	c := bashCall(id, command)
	c.Complete(result, 3)

	return c
}

func assistantMessage(parts ...part.Part) message.Message {
	return message.Message{Role: roles.Assistant, Parts: parts}
}

// renderPlain renders msg and strips styling, leaving the layout.
func renderPlain(t *testing.T, m Model, msg message.Message) string {
	t.Helper()

	rendered, lines := m.renderMessage(msg, m.width, 0)
	if got := strings.Count(rendered, "\n") + 1; rendered != "" && got != lines {
		t.Fatalf("renderMessage reported %d lines, rendered %d", lines, got)
	}

	return ansi.Strip(rendered)
}

// blocks splits plain output into blank-line separated blocks.
func blocks(plain string) []string {
	return strings.Split(plain, "\n\n")
}

func assertNoStackedBlankLines(t *testing.T, plain string) {
	t.Helper()

	if strings.Contains(plain, "\n\n\n") {
		t.Errorf("output has more than one blank line between blocks:\n%s", plain)
	}
}

func TestRenderAssistantThinkingAfterToolStartsOwnBlock(t *testing.T) {
	t.Parallel()

	msg := assistantMessage(
		contentPart("Let me look."),
		toolPart(completedBash("1", "ls", "file-a\nfile-b")),
		thinkingPart("Now I know the files."),
		contentPart("Done."),
	)

	plain := renderPlain(t, renderTestModel(true), msg)
	assertNoStackedBlankLines(t, plain)

	for line := range strings.SplitSeq(plain, "\n") {
		if strings.Contains(line, "file-b") && line != "file-b" {
			t.Errorf("tool result shares a line with following text: %q", line)
		}
	}

	got := blocks(plain)
	if len(got) != 4 {
		t.Fatalf("got %d blocks, want 4:\n%s", len(got), plain)
	}

	if !strings.HasPrefix(got[1], "✓ [Tool: Bash]") || !strings.HasSuffix(got[1], "file-b") {
		t.Errorf("tool block = %q, want heading followed by its result", got[1])
	}

	if want := thinkingLabel + "\nNow I know the files."; got[2] != want {
		t.Errorf("thinking block = %q, want %q", got[2], want)
	}

	if want := assistantLabel + " Done."; got[3] != want {
		t.Errorf("content after tool = %q, want %q", got[3], want)
	}
}

func TestRenderAssistantHiddenThinking(t *testing.T) {
	t.Parallel()

	parts := []part.Part{
		thinkingPart("Plan the search."),
		contentPart("Let me look."),
		toolPart(completedBash("1", "ls", "file-a")),
		thinkingPart("Now I know the files."),
		contentPart("Done."),
	}

	hidden := renderPlain(t, renderTestModel(false), assistantMessage(parts...))
	visible := renderPlain(t, renderTestModel(true), assistantMessage(parts...))

	assertNoStackedBlankLines(t, hidden)
	assertNoStackedBlankLines(t, visible)

	for _, leaked := range []string{thinkingLabel, "Plan the search.", "Now I know the files."} {
		if strings.Contains(hidden, leaked) {
			t.Errorf("hidden thinking leaked %q:\n%s", leaked, hidden)
		}
	}

	if got := blocks(hidden); len(got) != 3 {
		t.Errorf("hidden thinking left %d blocks, want 3 (content, tool, content):\n%s", len(got), hidden)
	}

	// Showing thinking adds blocks but must not move the assistant label.
	labelled := func(plain string) []string {
		var lines []string

		for line := range strings.SplitSeq(plain, "\n") {
			if strings.HasPrefix(line, assistantLabel) {
				lines = append(lines, line)
			}
		}

		return lines
	}

	if h, v := labelled(hidden), labelled(visible); !slices.Equal(h, v) {
		t.Errorf("assistant labels differ by thinking visibility: hidden %q, visible %q", h, v)
	}
}

func TestRenderAssistantLeadingThinking(t *testing.T) {
	t.Parallel()

	msg := assistantMessage(thinkingPart("  Plan.\n"), contentPart("Answer."))

	visible := blocks(renderPlain(t, renderTestModel(true), msg))
	if want := []string{thinkingLabel + "\nPlan.", assistantLabel + " Answer."}; !slices.Equal(visible, want) {
		t.Errorf("visible = %q, want %q", visible, want)
	}

	if hidden := renderPlain(t, renderTestModel(false), msg); hidden != assistantLabel+" Answer." {
		t.Errorf("hidden = %q, want only the labelled answer", hidden)
	}
}

func TestRenderAssistantAdjacentTools(t *testing.T) {
	t.Parallel()

	failed := bashCall("2", "false")
	failed.Fail(errors.New("exit status 1"), 2)

	msg := assistantMessage(
		toolPart(completedBash("1", "ls", "file-a")),
		toolPart(failed),
		toolPart(bashCall("3", "pwd")),
	)

	plain := renderPlain(t, renderTestModel(true), msg)
	assertNoStackedBlankLines(t, plain)

	got := blocks(plain)
	if len(got) != 3 {
		t.Fatalf("got %d blocks, want one per tool:\n%s", len(got), plain)
	}

	for i, icon := range []string{"✓ [Tool: Bash]", "✗ [Tool: Bash]", "○ [Tool: Bash]"} {
		if !strings.HasPrefix(got[i], icon) {
			t.Errorf("block %d = %q, want prefix %q", i, got[i], icon)
		}
	}

	// Blocks are blank-line separated, so a result inside the heading's block follows it directly.
	if !strings.HasSuffix(got[0], "\n[tokens: ~3]\n→ file-a") {
		t.Errorf("completed tool block does not end with its result: %q", got[0])
	}

	if !strings.Contains(got[1], "exit status 1") {
		t.Errorf("failed tool block lacks its error: %q", got[1])
	}

	if strings.Contains(plain, assistantLabel) {
		t.Errorf("tool-only message shows a dangling assistant label:\n%s", plain)
	}
}

func TestRenderAssistantError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  message.Message
		want string
	}{
		{
			name: "after content",
			msg: message.Message{
				Role:  roles.Assistant,
				Parts: []part.Part{contentPart("Partial.")},
				Error: errors.New("boom"),
			},
			want: assistantLabel + " Partial.\nError: boom",
		},
		{
			name: "cancelled without output",
			msg:  message.Message{Role: roles.Assistant, Error: context.Canceled},
			want: "Operation canceled by user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := renderPlain(t, renderTestModel(true), tt.msg); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderWrapsWithinWidth(t *testing.T) {
	t.Parallel()

	const width = 40

	m := renderTestModel(true)
	m.width = width

	running := bashCall("2", strings.Repeat("a", 3*width))
	running.MarkRunning()

	msgs := []message.Message{
		{Role: roles.User, Parts: []part.Part{contentPart(strings.Repeat("question words ", 8))}},
		assistantMessage(
			thinkingPart(strings.Repeat("considering options ", 8)),
			contentPart(strings.Repeat("answer words ", 8)),
			toolPart(completedBash("1", strings.Repeat("b", 2*width), strings.Repeat("c", 2*width))),
			toolPart(running),
		),
	}

	for _, msg := range msgs {
		rendered, _ := m.renderMessage(msg, width, 0)

		for line := range strings.SplitSeq(rendered, "\n") {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("line is %d cells wide, viewport is %d: %q", w, width, ansi.Strip(line))
			}
		}
	}
}

func TestRenderChatHistoryStreamingTransitions(t *testing.T) {
	t.Parallel()

	// Progress is shown by the activity row (see activity_test.go); the history only shows parts.
	m := renderTestModel(false)
	m.messages = message.Messages{{Role: roles.User, Parts: []part.Part{contentPart("hi")}}}
	m.currentMessage = &message.Message{Role: roles.Assistant}

	history := ansi.Strip(m.renderChatHistory())
	if history != "You: hi\n\n" {
		t.Errorf("empty streaming message adds to the history: %q", history)
	}

	m.currentMessage.Parts = append(m.currentMessage.Parts, thinkingPart("secret plan"))

	if got := ansi.Strip(m.renderChatHistory()); got != history {
		t.Errorf("hidden thinking must not change the history: %q", got)
	}

	m.messages = nil
	m.hints.Verbose = true

	tool := bashCall("1", "ls")
	tool.MarkRunning()
	m.currentMessage.Parts = append(m.currentMessage.Parts, toolPart(tool))

	got := ansi.Strip(m.renderChatHistory())
	if !strings.HasPrefix(got, thinkingLabel+"\nsecret plan\n\n") || !strings.Contains(got, "[Tool: Bash]") {
		t.Errorf("running tool does not follow thinking as its own block: %q", got)
	}

	// The tool completes and the next iteration starts thinking right away.
	m.currentMessage.Parts[1] = toolPart(completedBash("1", "ls", "file-a"))
	m.currentMessage.Parts = append(m.currentMessage.Parts, thinkingPart("next step"))

	got = ansi.Strip(m.renderChatHistory())
	assertNoStackedBlankLines(t, got)

	if !strings.HasSuffix(got, "file-a\n\n"+thinkingLabel+"\nnext step") {
		t.Errorf("thinking after a completed tool does not start its own block: %q", got)
	}
}

func TestRenderChatHistorySkipsInvisibleMessages(t *testing.T) {
	t.Parallel()

	m := renderTestModel(false)
	m.messages = message.Messages{
		{Role: roles.User, Parts: []part.Part{contentPart("hi")}},
		assistantMessage(thinkingPart("only hidden thoughts")),
		assistantMessage(contentPart("hello")),
	}

	plain := ansi.Strip(m.renderChatHistory())
	assertNoStackedBlankLines(t, plain)

	if want := "You: hi\n\n" + assistantLabel + " hello\n\n"; plain != want {
		t.Errorf("history = %q, want %q", plain, want)
	}
}

// Selection is painted per message from tracked line numbers, while the selection itself
// addresses lines of the rendered content. Selecting any one line must highlight exactly that line.
func TestRenderChatHistorySelectionTracksLines(t *testing.T) {
	t.Parallel()

	for _, streaming := range []bool{false, true} {
		m := renderTestModel(false)
		m.messages = message.Messages{
			{Role: roles.User, Parts: []part.Part{contentPart("hi")}},
			assistantMessage(thinkingPart("only hidden thoughts")),
			assistantMessage(
				contentPart("Let me look."),
				toolPart(completedBash("1", "ls", "file-a\nfile-b")),
				contentPart("Done."),
			),
		}
		m.processingMessages = []string{"accepted follow-up"}
		m.pendingMessages = []string{"queued follow-up"}

		if streaming {
			m.currentMessage = new(assistantMessage(contentPart("Working on it."), toolPart(bashCall("2", "pwd"))))
		}

		base := strings.Split(m.renderChatHistory(), "\n")

		for target := range base {
			sel := m
			sel.selection = selectionState{
				start:   Position{Col: 0, Line: target},
				end:     Position{Col: m.width, Line: target},
				enabled: true,
			}

			selected := strings.Split(sel.renderChatHistory(), "\n")

			for i := range base {
				want := i == target && ansi.StringWidth(base[i]) > 0
				if changed := base[i] != selected[i]; changed != want {
					t.Errorf("streaming=%v: selecting line %d changed line %d (%q): got %v, want %v",
						streaming, target, i, ansi.Strip(base[i]), changed, want)
				}
			}
		}
	}
}

func TestRenderRestoredHistory(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(message.Message{
		Role:     roles.Assistant,
		Thinking: "Check the tree.",
		Content:  "Listing files.",
		Calls:    []call.Call{{ID: "1", Name: "Bash", Arguments: map[string]any{"command": "ls"}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var restored message.Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	got := blocks(renderPlain(t, renderTestModel(true), restored))
	if len(got) != 3 {
		t.Fatalf("got %d blocks, want thinking, content, tool: %q", len(got), got)
	}

	if got[0] != thinkingLabel+"\nCheck the tree." || got[1] != assistantLabel+" Listing files." {
		t.Errorf("restored thinking/content = %q", got[:2])
	}

	// Restored calls have no result in the assistant message; they render as pending headings.
	if !strings.HasPrefix(got[2], "○ [Tool: Bash]") {
		t.Errorf("restored call = %q, want a pending tool heading", got[2])
	}
}
