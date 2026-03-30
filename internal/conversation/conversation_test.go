package conversation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/conversation"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// eventCollector records every event sent to it.
type eventCollector struct {
	events []ui.Event
}

// sink returns an EventSink function that appends events to the collector.
func (c *eventCollector) sink() conversation.EventSink {
	return func(event ui.Event) {
		c.events = append(c.events, event)
	}
}

// stubEstimate is a trivial token estimator for tests: 1 token per 4 bytes.
func stubEstimate(_ context.Context, text string) int {
	return len(text) / 4
}

// newBuilder is a test helper that creates a Builder with a nil sink.
func newBuilder(t *testing.T, systemPrompt string) *conversation.Builder {
	t.Helper()

	return conversation.NewBuilder(nil, systemPrompt, 0, stubEstimate)
}

func TestNewBuilder(t *testing.T) {
	t.Parallel()

	b := newBuilder(t, "you are helpful")

	if b.Len() != 1 {
		t.Errorf("Len() = %d, want 1", b.Len())
	}

	h := b.History()
	if h[0].Role != roles.System {
		t.Errorf("History()[0].Role = %q, want %q", h[0].Role, roles.System)
	}

	if h[0].Content != "you are helpful" {
		t.Errorf("History()[0].Content = %q, want %q", h[0].Content, "you are helpful")
	}
}

func TestAddUserMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "simple text",
			message: "hello world",
		},
		{
			name:    "empty string",
			message: "",
		},
		{
			name:    "multiline text",
			message: "line one\nline two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBuilder(t, "sys")
			b.AddUserMessage(context.Background(), tt.message, 0)

			if b.Len() != 2 {
				t.Errorf("Len() = %d, want 2", b.Len())
			}

			h := b.History()
			last := h[len(h)-1]

			if last.Role != roles.User {
				t.Errorf("last.Role = %q, want %q", last.Role, roles.User)
			}

			if last.Content != tt.message {
				t.Errorf("last.Content = %q, want %q", last.Content, tt.message)
			}
		})
	}
}

func TestAddUserMessage_EmitsEvent(t *testing.T) {
	t.Parallel()

	sink := &eventCollector{}
	b := conversation.NewBuilder(sink.sink(), "sys", 0, stubEstimate)
	b.AddUserMessage(context.Background(), "hi", 0)

	if len(sink.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(sink.events))
	}

	ev, ok := sink.events[0].(ui.MessageAdded)
	if !ok {
		t.Fatalf("event type = %T, want ui.MessageAdded", sink.events[0])
	}

	if ev.Message.Role != roles.User {
		t.Errorf("event.Message.Role = %q, want %q", ev.Message.Role, roles.User)
	}

	if ev.Message.Content != "hi" {
		t.Errorf("event.Message.Content = %q, want %q", ev.Message.Content, "hi")
	}
}

func TestAddAssistantMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "simple assistant reply",
			content: "I can help with that.",
		},
		{
			name:    "empty content",
			content: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBuilder(t, "sys")
			msg := message.Message{
				Role:    roles.Assistant,
				Content: tt.content,
			}
			b.AddAssistantMessage(msg)

			if b.Len() != 2 {
				t.Errorf("Len() = %d, want 2", b.Len())
			}

			h := b.History()
			last := h[len(h)-1]

			if last.Role != roles.Assistant {
				t.Errorf("last.Role = %q, want %q", last.Role, roles.Assistant)
			}

			if last.Content != tt.content {
				t.Errorf("last.Content = %q, want %q", last.Content, tt.content)
			}
		})
	}
}

func TestAddToolResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		toolName   string
		toolCallID string
		result     string
	}{
		{
			name:       "bash tool result",
			toolName:   "Bash",
			toolCallID: "call-123",
			result:     "exit code 0",
		},
		{
			name:       "read tool result",
			toolName:   "Read",
			toolCallID: "call-456",
			result:     "file contents here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBuilder(t, "sys")
			b.AddToolResult(context.Background(), tt.toolName, tt.toolCallID, tt.result, 0)

			if b.Len() != 2 {
				t.Errorf("Len() = %d, want 2", b.Len())
			}

			h := b.History()
			last := h[len(h)-1]

			if last.Role != roles.Tool {
				t.Errorf("last.Role = %q, want %q", last.Role, roles.Tool)
			}

			if last.ToolName != tt.toolName {
				t.Errorf("last.ToolName = %q, want %q", last.ToolName, tt.toolName)
			}

			if last.ToolCallID != tt.toolCallID {
				t.Errorf("last.ToolCallID = %q, want %q", last.ToolCallID, tt.toolCallID)
			}

			if last.Content != tt.result {
				t.Errorf("last.Content = %q, want %q", last.Content, tt.result)
			}
		})
	}
}

func TestStreamingFlow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		chunks []string
		want   string
	}{
		{
			name:   "two chunks merged",
			chunks: []string{"hello", " world"},
			want:   "hello world",
		},
		{
			name:   "single chunk",
			chunks: []string{"only"},
			want:   "only",
		},
		{
			name:   "three chunks",
			chunks: []string{"a", "b", "c"},
			want:   "abc",
		},
		{
			name:   "empty chunks skipped",
			chunks: []string{"start", "", "end"},
			want:   "startend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBuilder(t, "sys")
			b.StartAssistant()

			for _, chunk := range tt.chunks {
				b.AppendContent(chunk)
			}

			// FinalizeAssistant does not write to history in Builder —
			// only AddAssistantMessage does. Verify streaming state via
			// the event sink instead.
			sink := &eventCollector{}
			b2 := conversation.NewBuilder(sink.sink(), "sys", 0, stubEstimate)
			b2.StartAssistant()

			for _, chunk := range tt.chunks {
				b2.AppendContent(chunk)
			}

			b2.FinalizeAssistant()

			// Find MessageFinalized event
			var finalized *ui.MessageFinalized

			for _, ev := range sink.events {
				if mf, ok := ev.(ui.MessageFinalized); ok {
					finalized = &mf
				}
			}

			if finalized == nil {
				t.Fatal("no MessageFinalized event emitted")
			}

			// Collect text from content parts
			var got strings.Builder

			for _, p := range finalized.Message.Parts {
				if p.IsContent() {
					got.WriteString(p.Text)
				}
			}

			if got.String() != tt.want {
				t.Errorf("streamed content = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestUpdateSystemPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial string
		updated string
		addMsgs int // user messages to add before updating
		wantLen int
	}{
		{
			name:    "updates prompt text",
			initial: "old prompt",
			updated: "new prompt",
			addMsgs: 0,
			wantLen: 1,
		},
		{
			name:    "preserves history length",
			initial: "sys",
			updated: "updated sys",
			addMsgs: 2,
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBuilder(t, tt.initial)

			for range tt.addMsgs {
				b.AddUserMessage(context.Background(), "msg", 0)
			}

			b.UpdateSystemPrompt(context.Background(), tt.updated)

			if b.Len() != tt.wantLen {
				t.Errorf("Len() = %d, want %d", b.Len(), tt.wantLen)
			}

			h := b.History()
			if h[0].Content != tt.updated {
				t.Errorf("History()[0].Content = %q, want %q", h[0].Content, tt.updated)
			}

			if h[0].Role != roles.System {
				t.Errorf("History()[0].Role = %q, want %q", h[0].Role, roles.System)
			}
		})
	}
}

func TestRestore(t *testing.T) {
	t.Parallel()

	b := newBuilder(t, "old sys")
	b.AddUserMessage(context.Background(), "original message", 0)

	replacement := message.New(
		message.Message{Role: roles.System, Content: "new sys"},
		message.Message{Role: roles.User, Content: "restored user msg"},
	)

	b.Restore(replacement)

	if b.Len() != 2 {
		t.Errorf("Len() = %d, want 2", b.Len())
	}

	h := b.History()

	if h[0].Role != roles.System {
		t.Errorf("h[0].Role = %q, want %q", h[0].Role, roles.System)
	}

	if h[0].Content != "new sys" {
		t.Errorf("h[0].Content = %q, want %q", h[0].Content, "new sys")
	}

	if h[1].Content != "restored user msg" {
		t.Errorf("h[1].Content = %q, want %q", h[1].Content, "restored user msg")
	}
}

func TestClear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addMsgs int
	}{
		{
			name:    "clears messages leaving only system",
			addMsgs: 3,
		},
		{
			name:    "clear with no extra messages",
			addMsgs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBuilder(t, "system prompt")

			for range tt.addMsgs {
				b.AddUserMessage(context.Background(), "msg", 0)
			}

			b.Clear()

			if b.Len() != 1 {
				t.Errorf("Len() = %d, want 1", b.Len())
			}

			h := b.History()
			if h[0].Role != roles.System {
				t.Errorf("History()[0].Role = %q, want %q", h[0].Role, roles.System)
			}

			if h[0].Content != "system prompt" {
				t.Errorf("History()[0].Content = %q, want %q", h[0].Content, "system prompt")
			}
		})
	}
}

func TestDropN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addMsgs int
		drop    int
		wantLen int
	}{
		{
			name:    "drop one of three",
			addMsgs: 2,
			drop:    1,
			wantLen: 2,
		},
		{
			name:    "drop two of three",
			addMsgs: 2,
			drop:    2,
			wantLen: 1,
		},
		{
			name:    "drop zero is no-op",
			addMsgs: 2,
			drop:    0,
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBuilder(t, "sys")

			for range tt.addMsgs {
				b.AddUserMessage(context.Background(), "msg", 0)
			}

			b.DropN(tt.drop)

			if b.Len() != tt.wantLen {
				t.Errorf("Len() = %d, want %d", b.Len(), tt.wantLen)
			}
		})
	}
}

func TestInjectMessage_AndEjectSyntheticMessages(t *testing.T) {
	t.Parallel()

	b := newBuilder(t, "sys")
	b.AddUserMessage(context.Background(), "real message", 0)
	b.InjectMessage(context.Background(), roles.User, "synthetic content")

	// After injection: system + real user + synthetic = 3
	if b.Len() != 3 {
		t.Errorf("Len() after inject = %d, want 3", b.Len())
	}

	// Verify the synthetic message is marked synthetic
	h := b.History()
	last := h[len(h)-1]

	if !last.IsSynthetic() {
		t.Errorf("injected message IsSynthetic() = false, want true")
	}

	if last.Content != "synthetic content" {
		t.Errorf("injected message Content = %q, want %q", last.Content, "synthetic content")
	}

	// Eject removes synthetic messages
	b.EjectSyntheticMessages()

	if b.Len() != 2 {
		t.Errorf("Len() after eject = %d, want 2", b.Len())
	}

	h = b.History()
	for _, msg := range h {
		if msg.IsSynthetic() {
			t.Errorf("found synthetic message after eject: %q", msg.Content)
		}
	}
}

func TestTrimDuplicateSynthetics(t *testing.T) {
	t.Parallel()

	b := newBuilder(t, "sys")
	b.InjectMessage(context.Background(), roles.User, "loop detected")
	b.InjectMessage(context.Background(), roles.User, "loop detected") // duplicate

	// system + 2 synthetics = 3
	if b.Len() != 3 {
		t.Errorf("Len() before trim = %d, want 3", b.Len())
	}

	b.TrimDuplicateSynthetics()

	// After trim: system + 1 synthetic = 2
	if b.Len() != 2 {
		t.Errorf("Len() after trim = %d, want 2", b.Len())
	}

	h := b.History()
	last := h[len(h)-1]

	if !last.IsSynthetic() {
		t.Errorf("remaining message IsSynthetic() = false, want true")
	}

	if last.Content != "loop detected" {
		t.Errorf("remaining message Content = %q, want %q", last.Content, "loop detected")
	}
}

func TestHistory_ReturnedSliceIsSnapshot(t *testing.T) {
	t.Parallel()

	b := newBuilder(t, "sys")
	b.AddUserMessage(context.Background(), "first", 0)

	h := b.History()
	originalLen := len(h)

	// Mutate the returned slice by appending a dummy element.
	// This must not affect the builder's internal state.
	_ = append(h, message.Message{Role: roles.User, Content: "injected"})

	b.AddUserMessage(context.Background(), "second", 0)

	if b.Len() != originalLen+1 {
		t.Errorf("Len() = %d, want %d; builder state corrupted by external slice mutation", b.Len(), originalLen+1)
	}
}

func TestNilSink_DoesNotPanic(t *testing.T) {
	t.Parallel()

	// All mutating operations must be safe when sink is nil.
	b := conversation.NewBuilder(nil, "sys", 0, stubEstimate)
	b.AddUserMessage(context.Background(), "u", 0)

	msg := message.Message{Role: roles.Assistant, Content: "a"}
	b.AddAssistantMessage(msg)
	b.AddToolResult(context.Background(), "tool", "id", "result", 0)
	b.StartAssistant()
	b.AppendContent("streaming")
	b.AppendThinking("thinking")
	b.FinalizeAssistant()
	b.InjectMessage(context.Background(), roles.User, "synthetic")
	b.EjectSyntheticMessages()
}
