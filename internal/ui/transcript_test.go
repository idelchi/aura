package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/part"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

func TestTranscriptStreamsWithoutRepeatingOrClipping(t *testing.T) {
	var output bytes.Buffer
	log := NewTranscript(&output)
	record := func(e Event) {
		t.Helper()
		if err := log.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	record(DisplayHintsChanged{Hints: DisplayHints{Verbose: false}})
	record(MessageStarted{MessageID: "a"})
	thinking := part.Part{Type: part.Thinking, Text: "Inspect"}
	record(MessagePartAdded{MessageID: "a", Part: thinking})
	thinking.Text += " the files."
	record(MessagePartUpdated{MessageID: "a", PartIndex: 0, Part: thinking})
	if !strings.Contains(output.String(), "Inspect the files.") {
		t.Fatal("thinking was not streamed before finalization, independently of visibility")
	}
	tool := &call.Call{ID: "tool", Name: "Bash", State: call.Pending}
	command := "printf " + strings.Repeat("x", 450) + " command-end"
	tool.SetArgs(map[string]any{"command": command})
	p := part.Part{Type: part.Tool, Call: tool}
	record(MessagePartAdded{MessageID: "a", Part: p.Snapshot()})
	tool.MarkRunning()
	record(MessagePartUpdated{MessageID: "a", PartIndex: 1, Part: p.Snapshot()})
	tool.Complete(strings.Repeat("line\n", 150)+"result-end", 151)
	record(MessagePartUpdated{MessageID: "a", PartIndex: 1, Part: p.Snapshot()})
	record(MessagePartUpdated{MessageID: "a", PartIndex: 1, Part: p.Snapshot()})
	after := part.Part{Type: part.Thinking, Text: "Found the result."}
	record(MessagePartAdded{MessageID: "a", Part: after})
	answer := part.Part{Type: part.Content, Text: "Done."}
	record(MessagePartAdded{MessageID: "a", Part: answer})
	record(MessageFinalized{Message: message.Message{
		ID: "a", Role: roles.Assistant, Parts: []part.Part{thinking, p.Snapshot(), after, answer},
	}})
	text := output.String()
	for _, want := range []string{"Inspect the files.", command, "result-end", "Found the result.", "Done."} {
		if strings.Count(text, want) != 1 {
			t.Errorf("expected exactly one untruncated %q in transcript", want)
		}
	}
	if strings.Contains(text, "result-endFound") || strings.Count(text, "[Tool: Bash]") != 1 {
		t.Fatal("tool/thinking sections were joined or tool header repeated")
	}
}

func TestTranscriptDoesNotAnswerInteractiveEvents(t *testing.T) {
	var output bytes.Buffer
	log := NewTranscript(&output)
	flush := make(chan struct{})
	ask := make(chan string, 1)
	confirm := make(chan ConfirmAction, 1)
	for _, e := range []Event{
		AskRequired{Question: "Choose a file", Response: ask},
		ToolConfirmRequired{ToolName: "Bash", Detail: "touch file", Response: confirm},
		Flush{Done: flush},
	} {
		if err := log.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-flush:
		t.Fatal("recorder acknowledged a UI flush")
	default:
	}
	if len(ask) != 0 || len(confirm) != 0 {
		t.Fatal("recorder answered an interactive prompt")
	}
	if !strings.Contains(output.String(), "Choose a file") || !strings.Contains(output.String(), "touch file") {
		t.Fatal("interactive prompts were omitted")
	}
}

func TestTranscriptRecordsErrorsOnceAndPreservesClearHistory(t *testing.T) {
	var output bytes.Buffer
	log := NewTranscript(&output)
	err := errors.New("provider disconnected")
	for _, e := range []Event{
		CommandResult{Command: "/help", Message: "help text"},
		CommandResult{Command: "/clear", Clear: true},
		MessageFinalized{Message: message.Message{ID: "a", Role: roles.Assistant, Error: err}},
		AssistantDone{Error: err},
	} {
		if err := log.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(output.String(), "help text") || !strings.Contains(output.String(), "Screen cleared") {
		t.Fatal("screen clear discarded transcript history")
	}
	if strings.Count(output.String(), err.Error()) != 1 {
		t.Fatal("duplicate error in transcript")
	}
}

type failedTranscriptWriter struct{ err error }

func (w failedTranscriptWriter) Write([]byte) (int, error) { return 0, w.err }

func TestTranscriptReportsWriteFailure(t *testing.T) {
	want := errors.New("disk full")
	log := NewTranscript(failedTranscriptWriter{err: want})
	if err := log.Record(CommandResult{Message: "hello"}); !errors.Is(err, want) {
		t.Fatalf("lost transcript write error: %v", err)
	}
}

func TestTranscriptRestoresMessagesWithoutDisplayIDs(t *testing.T) {
	var messages message.Messages
	// Session JSON does not preserve display IDs or transient tool states.
	if err := json.Unmarshal([]byte(`[
		{"Role":"assistant","Content":"An earlier long answer."},
		{"Role":"user","Content":"And next?"},
		{"Role":"assistant","Content":"Short.","calls":[{"ID":"t","Name":"Read","Arguments":{"file_path":"note.md"}}]}
	]`), &messages); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := NewTranscript(&output).Record(SessionRestored{Messages: messages}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"An earlier long answer.", "And next?", "Short.", "[Tool: Read]", "note.md"} {
		if strings.Count(output.String(), want) != 1 {
			t.Errorf("restored transcript lost or duplicated %q: %s", want, &output)
		}
	}
}
