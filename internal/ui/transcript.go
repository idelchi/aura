package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// Transcript records conversation events without terminal redraws or interactive
// side effects. Record must be called serially, before the UI handles each event.
// Thinking is recorded independently of its visibility on screen.
type Transcript struct {
	writer    io.Writer
	status    Status
	messageID string
	parts     []transcriptPart
	section   string
	lastError string
}

type transcriptPart struct {
	textLength int
	header     bool
	finished   bool
}

// NewTranscript creates a streaming, plain-text conversation recorder.
func NewTranscript(w io.Writer) *Transcript { return &Transcript{writer: w} }

// Record writes only new material. It never answers dialogs, acknowledges Flush,
// or submits input: those responsibilities remain with the interactive UI.
func (t *Transcript) Record(event Event) error {
	var b strings.Builder

	switch e := event.(type) {
	case StatusChanged:
		t.status = e.Status
	case MessageStarted:
		t.start(e.MessageID)
	case MessageAdded:
		t.message(&b, e.Message)
	case MessagePartAdded:
		t.start(e.MessageID)
		t.part(&b, len(t.parts), e.Part, false)
	case MessagePartUpdated:
		t.start(e.MessageID)
		t.part(&b, e.PartIndex, e.Part, false)
	case MessageFinalized:
		t.message(&b, e.Message)
		if e.Message.Error != nil {
			t.reportError(&b, e.Message.Error)
		}
	case AssistantDone:
		if e.Cancelled {
			t.block(&b, "(cancelled)")
		} else if e.Error != nil {
			t.reportError(&b, e.Error)
		}
		t.lastError = ""
	case SessionRestored:
		t.block(&b, "--- Restored conversation ---")
		for _, msg := range e.Messages {
			// Stored messages need not have display IDs. Each historical message
			// is independent even when its ID matches the preceding one.
			t.messageID, t.section, t.parts = "", "", nil
			t.message(&b, msg)
		}
	case SyntheticInjected:
		t.block(&b, e.Header+"\n"+e.Content)
	case CommandResult:
		if e.Clear {
			t.block(&b, "--- Screen cleared ---")
		}
		if e.Command != "" {
			t.block(&b, t.status.UserPrompt()+e.Command)
		}
		if e.Message != "" {
			t.block(&b, strings.TrimSpace(e.Message))
		}
		if e.Error != nil {
			t.block(&b, "Error: "+e.Error.Error())
		}
	case ToolConfirmRequired:
		t.block(&b, fmt.Sprintf("[Confirmation required: %s]\n%s\n%s", e.ToolName, e.Description, e.Detail))
	case AskRequired:
		t.block(&b, "[Question]\n"+e.Question)
		for _, option := range e.Options {
			fmt.Fprintf(&b, "- %s: %s\n", option.Label, option.Description)
		}
	case PickerOpen:
		t.block(&b, e.Display())
	}

	if b.Len() == 0 {
		return nil
	}

	_, err := io.WriteString(t.writer, b.String())
	return err
}

func (t *Transcript) start(id string) {
	if id != t.messageID {
		t.messageID = id
		t.parts = nil
		t.section = ""
	}
}

func (t *Transcript) block(b *strings.Builder, text string) {
	fmt.Fprintf(b, "\n%s\n", text)
	t.section = ""
}

func (t *Transcript) reportError(b *strings.Builder, err error) {
	if text := err.Error(); text != t.lastError {
		t.block(b, "Error: "+text)
		t.lastError = text
	}
}

func (t *Transcript) message(b *strings.Builder, msg message.Message) {
	switch {
	case msg.IsBookmark():
		t.block(b, "--- "+strings.TrimSpace(msg.Content)+" ---")
	case msg.IsDisplayOnly():
		t.block(b, strings.TrimSpace(msg.Content))
	case msg.Role == roles.User:
		t.block(b, t.status.UserPrompt()+strings.TrimSpace(msg.Content))
	case msg.Role == roles.Assistant:
		t.start(msg.ID)
		for i, p := range msg.Parts {
			t.part(b, i, p, true)
		}
	}
}

func (t *Transcript) part(b *strings.Builder, index int, p part.Part, final bool) {
	if index < 0 {
		return
	}
	for len(t.parts) <= index {
		t.parts = append(t.parts, transcriptPart{})
	}
	seen := &t.parts[index]
	key := fmt.Sprintf("%s:%d", t.messageID, index)

	switch p.Type {
	case part.Thinking, part.Content:
		if len(p.Text) <= seen.textLength {
			return
		}
		if t.section != key {
			label := t.status.AssistantPrompt()
			if p.IsThinking() {
				label += "[Thinking]"
			}
			fmt.Fprintf(b, "\n\n%s\n", strings.TrimSpace(label))
			t.section = key
		}
		b.WriteString(p.Text[seen.textLength:])
		seen.textLength = len(p.Text)
	case part.Tool:
		if p.Call == nil || (p.Call.State == call.Pending && !final) {
			return
		}
		if !seen.header {
			header, args, _ := p.Call.DisplayTranscript()
			t.block(b, header+"\n"+args)
			seen.header = true
		}
		if !seen.finished && (p.Call.State == call.Complete || p.Call.State == call.Error) {
			_, _, result := p.Call.DisplayTranscript()
			t.block(b, result)
			seen.finished = true
		}
	}
}
