package message_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// helpers

func userMsg(content string) message.Message {
	return message.Message{Role: roles.User, Content: content}
}

func assistantMsg(content string) message.Message {
	return message.Message{Role: roles.Assistant, Content: content}
}

func systemMsg(content string) message.Message {
	return message.Message{Role: roles.System, Content: content}
}

func toolMsg(name, content string) message.Message {
	return message.Message{Role: roles.Tool, ToolName: name, Content: content}
}

func syntheticMsg(content string) message.Message {
	return message.Message{Role: roles.User, Content: content, Type: message.Synthetic}
}

func displayOnlyMsg(content string) message.Message {
	return message.Message{Role: roles.User, Content: content, Type: message.DisplayOnly}
}

func bookmarkMsg(label string) message.Message {
	return message.Message{Role: roles.System, Content: label, Type: message.Bookmark}
}

func metadataMsg(content string) message.Message {
	return message.Message{Role: roles.System, Content: content, Type: message.Metadata}
}

// assertLen checks the messages slice has exactly n elements.
func assertLen(t *testing.T, ms message.Messages, n int) {
	t.Helper()

	if len(ms) != n {
		t.Fatalf("len(Messages) = %d, want %d", len(ms), n)
	}
}

// -----------------------------------------------------------------------------
// New + Add
// -----------------------------------------------------------------------------

func TestNew_And_Add(t *testing.T) {
	t.Parallel()

	t.Run("new with initial messages", func(t *testing.T) {
		t.Parallel()

		ms := message.New(userMsg("hello"), assistantMsg("hi"))
		assertLen(t, ms, 2)
	})

	t.Run("new empty then add", func(t *testing.T) {
		t.Parallel()

		ms := message.New()
		ms.Add(userMsg("first"))
		ms.Add(assistantMsg("second"))

		assertLen(t, ms, 2)

		if ms[0].Content != "first" {
			t.Errorf("Messages[0].Content = %q, want %q", ms[0].Content, "first")
		}

		if ms[1].Content != "second" {
			t.Errorf("Messages[1].Content = %q, want %q", ms[1].Content, "second")
		}
	})

	t.Run("add preserves order", func(t *testing.T) {
		t.Parallel()

		ms := message.New(systemMsg("system"))
		ms.Add(userMsg("user1"))
		ms.Add(assistantMsg("asst1"))

		assertLen(t, ms, 3)

		if ms[0].Role != roles.System {
			t.Errorf("Messages[0].Role = %q, want %q", ms[0].Role, roles.System)
		}
	})
}

// -----------------------------------------------------------------------------
// Last
// -----------------------------------------------------------------------------

func TestLast(t *testing.T) {
	t.Parallel()

	t.Run("returns pointer to last message", func(t *testing.T) {
		t.Parallel()

		ms := message.New(userMsg("first"), assistantMsg("last"))
		got := ms.Last()

		if got == nil {
			t.Fatal("Last() = nil, want non-nil")
		}

		if got.Content != "last" {
			t.Errorf("Last().Content = %q, want %q", got.Content, "last")
		}

		if got.Role != roles.Assistant {
			t.Errorf("Last().Role = %q, want %q", got.Role, roles.Assistant)
		}
	})

	t.Run("mutation via pointer updates collection", func(t *testing.T) {
		t.Parallel()

		ms := message.New(userMsg("original"))

		last := ms.Last()
		if last == nil {
			t.Fatal("Last() = nil, want non-nil")
		}

		last.Content = "mutated"

		if ms[0].Content != "mutated" {
			t.Errorf("Messages[0].Content = %q after mutation via Last(), want %q", ms[0].Content, "mutated")
		}
	})

	t.Run("nil on empty collection", func(t *testing.T) {
		t.Parallel()

		ms := message.New()
		got := ms.Last()

		if got != nil {
			t.Errorf("Last() = %v, want nil for empty Messages", got)
		}
	})
}

// -----------------------------------------------------------------------------
// Clear
// -----------------------------------------------------------------------------

func TestClear(t *testing.T) {
	t.Parallel()

	t.Run("keeps only first message", func(t *testing.T) {
		t.Parallel()

		ms := message.New(systemMsg("system"), userMsg("a"), assistantMsg("b"))
		ms.Clear()

		assertLen(t, ms, 1)

		if ms[0].Role != roles.System {
			t.Errorf("Messages[0].Role = %q, want %q", ms[0].Role, roles.System)
		}
	})

	t.Run("single message collection unchanged", func(t *testing.T) {
		t.Parallel()

		ms := message.New(systemMsg("only"))
		ms.Clear()

		assertLen(t, ms, 1)

		if ms[0].Content != "only" {
			t.Errorf("Messages[0].Content = %q, want %q", ms[0].Content, "only")
		}
	})

	t.Run("empty collection unchanged", func(t *testing.T) {
		t.Parallel()

		ms := message.New()
		ms.Clear()

		assertLen(t, ms, 0)
	})
}

// -----------------------------------------------------------------------------
// KeepFirstByRole
// -----------------------------------------------------------------------------

func TestKeepFirstByRole(t *testing.T) {
	t.Parallel()

	t.Run("keeps only first user message removes rest", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			systemMsg("system"),
			userMsg("first user"),
			assistantMsg("reply"),
			userMsg("second user"),
			userMsg("third user"),
		)
		ms.KeepFirstByRole(roles.User)

		// Should retain system, first user, assistant — remove second and third user.
		assertLen(t, ms, 3)

		userCount := 0

		for _, m := range ms {
			if m.Role == roles.User {
				userCount++

				if m.Content != "first user" {
					t.Errorf("kept user msg Content = %q, want %q", m.Content, "first user")
				}
			}
		}

		if userCount != 1 {
			t.Errorf("user message count = %d, want 1", userCount)
		}
	})

	t.Run("other roles are untouched", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			systemMsg("sys"),
			assistantMsg("asst1"),
			assistantMsg("asst2"),
		)
		ms.KeepFirstByRole(roles.User) // no user messages — nothing removed

		assertLen(t, ms, 3)
	})

	t.Run("no messages of target role is no-op", func(t *testing.T) {
		t.Parallel()

		ms := message.New(systemMsg("sys"), assistantMsg("asst"))
		ms.KeepFirstByRole(roles.User)

		assertLen(t, ms, 2)
	})
}

// -----------------------------------------------------------------------------
// DropN
// -----------------------------------------------------------------------------

func TestDropN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		initial  message.Messages
		n        int
		wantLen  int
		wantLast string // content of last remaining message, if any
	}{
		{
			name:     "drop zero is no-op",
			initial:  message.New(userMsg("a"), userMsg("b"), userMsg("c")),
			n:        0,
			wantLen:  3,
			wantLast: "c",
		},
		{
			name:     "drop some from end",
			initial:  message.New(userMsg("a"), userMsg("b"), userMsg("c")),
			n:        2,
			wantLen:  1,
			wantLast: "a",
		},
		{
			name:    "drop exactly all",
			initial: message.New(userMsg("a"), userMsg("b")),
			n:       2,
			wantLen: 0,
		},
		{
			name:    "drop more than len clears all",
			initial: message.New(userMsg("a"), userMsg("b")),
			n:       100,
			wantLen: 0,
		},
		{
			name:    "drop from empty is no-op",
			initial: message.New(),
			n:       5,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ms := tt.initial
			ms.DropN(tt.n)

			assertLen(t, ms, tt.wantLen)

			if tt.wantLast != "" {
				last := ms.Last()
				if last == nil {
					t.Fatalf("Last() = nil, want message with content %q", tt.wantLast)
				}

				if last.Content != tt.wantLast {
					t.Errorf("Last().Content = %q, want %q", last.Content, tt.wantLast)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// WithoutSyntheticMessages
// -----------------------------------------------------------------------------

func TestWithoutSyntheticMessages(t *testing.T) {
	t.Parallel()

	t.Run("filters out synthetic messages", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("real"),
			syntheticMsg("synthetic one"),
			assistantMsg("also real"),
			syntheticMsg("synthetic two"),
		)

		got := ms.WithoutSyntheticMessages()

		assertLen(t, got, 2)

		for _, m := range got {
			if m.IsSynthetic() {
				t.Errorf("WithoutSyntheticMessages() result contains synthetic message: %q", m.Content)
			}
		}
	})

	t.Run("returns new slice does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := message.New(userMsg("real"), syntheticMsg("synthetic"))
		original := len(ms)

		_ = ms.WithoutSyntheticMessages()

		if len(ms) != original {
			t.Errorf("original Messages len = %d after WithoutSyntheticMessages(), want %d", len(ms), original)
		}
	})

	t.Run("no synthetics returns full copy", func(t *testing.T) {
		t.Parallel()

		ms := message.New(userMsg("a"), assistantMsg("b"))
		got := ms.WithoutSyntheticMessages()

		assertLen(t, got, 2)
	})

	t.Run("all synthetics returns empty", func(t *testing.T) {
		t.Parallel()

		ms := message.New(syntheticMsg("x"), syntheticMsg("y"))
		got := ms.WithoutSyntheticMessages()

		assertLen(t, got, 0)
	})
}

// -----------------------------------------------------------------------------
// TrimDuplicateSynthetics
// -----------------------------------------------------------------------------

func TestTrimDuplicateSynthetics(t *testing.T) {
	t.Parallel()

	t.Run("keeps most recent duplicate synthetic", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			syntheticMsg("repeated"),
			userMsg("interleaved"),
			syntheticMsg("repeated"),
		)
		ms.TrimDuplicateSynthetics()

		// The first "repeated" synthetic is removed; the second (most recent) stays.
		// After deletion the slice is compacted: [user("interleaved"), synthetic("repeated")].
		syntheticCount := 0

		for _, m := range ms {
			if m.IsSynthetic() && m.Content == "repeated" {
				syntheticCount++
			}
		}

		if syntheticCount != 1 {
			t.Errorf("synthetic count = %d, want 1", syntheticCount)
		}

		// The user message must still be present.
		userFound := false

		for _, m := range ms {
			if m.Role == roles.User && !m.IsSynthetic() {
				userFound = true
			}
		}

		if !userFound {
			t.Error("non-synthetic user message was removed by TrimDuplicateSynthetics")
		}

		// Overall length: 1 user + 1 synthetic = 2.
		assertLen(t, ms, 2)
	})

	t.Run("non-synthetic messages are never removed", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("user"),
			assistantMsg("asst"),
			systemMsg("sys"),
		)
		original := len(ms)
		ms.TrimDuplicateSynthetics()

		assertLen(t, ms, original)
	})

	t.Run("unique synthetics are not removed", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			syntheticMsg("unique one"),
			syntheticMsg("unique two"),
		)
		ms.TrimDuplicateSynthetics()

		assertLen(t, ms, 2)
	})

	t.Run("multiple duplicates each keep only most recent", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			syntheticMsg("alpha"),
			syntheticMsg("beta"),
			syntheticMsg("alpha"),
			syntheticMsg("beta"),
		)
		ms.TrimDuplicateSynthetics()

		assertLen(t, ms, 2)
	})
}

// -----------------------------------------------------------------------------
// WithoutThinking
// -----------------------------------------------------------------------------

func TestWithoutThinking(t *testing.T) {
	t.Parallel()

	t.Run("clears Thinking and ThinkingSignature fields", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			message.Message{
				Role:              roles.Assistant,
				Content:           "the answer",
				Thinking:          "my reasoning",
				ThinkingSignature: "sig123",
			},
			userMsg("follow up"),
		)

		got := ms.WithoutThinking()

		assertLen(t, got, 2)

		for _, m := range got {
			if m.Thinking != "" {
				t.Errorf("WithoutThinking() msg.Thinking = %q, want empty", m.Thinking)
			}

			if m.ThinkingSignature != "" {
				t.Errorf("WithoutThinking() msg.ThinkingSignature = %q, want empty", m.ThinkingSignature)
			}
		}
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := message.New(message.Message{
			Role:     roles.Assistant,
			Thinking: "keep this",
		})

		_ = ms.WithoutThinking()

		if ms[0].Thinking != "keep this" {
			t.Errorf("original Messages[0].Thinking = %q after WithoutThinking(), want %q",
				ms[0].Thinking, "keep this")
		}
	})

	t.Run("content field is preserved", func(t *testing.T) {
		t.Parallel()

		ms := message.New(message.Message{
			Role:     roles.Assistant,
			Content:  "important content",
			Thinking: "scratch work",
		})

		got := ms.WithoutThinking()

		if got[0].Content != "important content" {
			t.Errorf("WithoutThinking() Content = %q, want %q", got[0].Content, "important content")
		}
	})
}

// -----------------------------------------------------------------------------
// Truncated
// -----------------------------------------------------------------------------

func TestTruncated(t *testing.T) {
	t.Parallel()

	t.Run("tool results beyond maxLen get truncated", func(t *testing.T) {
		t.Parallel()

		longContent := strings.Repeat("x", 500)
		ms := message.New(
			userMsg("question"),
			toolMsg("read_file", longContent),
		)

		got := ms.Truncated(100)

		// The tool message content should be shorter than the original.
		var toolContent string

		for _, m := range got {
			if m.Role == roles.Tool {
				toolContent = m.Content
			}
		}

		if len(toolContent) >= len(longContent) {
			t.Errorf("tool content length = %d, want < %d after truncation", len(toolContent), len(longContent))
		}
	})

	t.Run("non-tool messages are unchanged", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("original user content"),
			assistantMsg("original assistant content"),
		)

		got := ms.Truncated(10)

		for _, m := range got {
			switch m.Role {
			case roles.User:
				if m.Content != "original user content" {
					t.Errorf("user Content = %q, want unchanged", m.Content)
				}
			case roles.Assistant:
				if m.Content != "original assistant content" {
					t.Errorf("assistant Content = %q, want unchanged", m.Content)
				}
			}
		}
	})

	t.Run("short tool results are not truncated", func(t *testing.T) {
		t.Parallel()

		ms := message.New(toolMsg("tool", "short"))
		got := ms.Truncated(1000)

		if got[0].Content != "short" {
			t.Errorf("Content = %q, want %q (no truncation for short content)", got[0].Content, "short")
		}
	})

	t.Run("synthetic messages are stripped", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("real"),
			syntheticMsg("synthetic"),
		)

		got := ms.WithoutSyntheticMessages().Truncated(1000)

		for _, m := range got {
			if m.IsSynthetic() {
				t.Errorf("Truncated() result contains synthetic message: %q", m.Content)
			}
		}
	})
}

// -----------------------------------------------------------------------------
// Render (Messages.Render)
// -----------------------------------------------------------------------------

func TestMessagesRender(t *testing.T) {
	t.Parallel()

	t.Run("produces non-empty string for non-empty messages", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			systemMsg("you are helpful"),
			userMsg("hello"),
			assistantMsg("hi there"),
		)

		got := ms.Render()

		if got == "" {
			t.Error("Render() = empty string, want non-empty for non-empty Messages")
		}
	})

	t.Run("contains role-prefixed content", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("test input"),
			assistantMsg("test output"),
		)

		got := ms.Render()

		if !strings.Contains(got, "test input") {
			t.Errorf("Render() = %q, want it to contain %q", got, "test input")
		}

		if !strings.Contains(got, "test output") {
			t.Errorf("Render() = %q, want it to contain %q", got, "test output")
		}
	})

	t.Run("empty messages renders empty string", func(t *testing.T) {
		t.Parallel()

		ms := message.New()
		got := ms.Render()

		if got != "" {
			t.Errorf("Render() = %q, want empty string for empty Messages", got)
		}
	})

	t.Run("tool message content appears in render", func(t *testing.T) {
		t.Parallel()

		ms := message.New(toolMsg("my_tool", "tool output here"))
		got := ms.Render()

		if !strings.Contains(got, "tool output here") {
			t.Errorf("Render() = %q, want it to contain %q", got, "tool output here")
		}

		if !strings.Contains(got, "my_tool") {
			t.Errorf("Render() = %q, want it to contain tool name %q", got, "my_tool")
		}
	})
}

// -----------------------------------------------------------------------------
// WithoutInternalMessages
// -----------------------------------------------------------------------------

func TestWithoutInternalMessages(t *testing.T) {
	t.Parallel()

	t.Run("filters out internal types", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("real"),
			displayOnlyMsg("notice"),
			assistantMsg("also real"),
			bookmarkMsg("compacted"),
			metadataMsg(`{"key":"val"}`),
		)

		got := ms.WithoutInternalMessages()

		assertLen(t, got, 2)

		for _, m := range got {
			if m.IsInternal() {
				t.Errorf("WithoutInternalMessages() result contains internal message: %q", m.Content)
			}
		}
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := message.New(userMsg("real"), bookmarkMsg("mark"))
		original := len(ms)

		_ = ms.WithoutInternalMessages()

		if len(ms) != original {
			t.Errorf("original Messages len = %d after WithoutInternalMessages(), want %d", len(ms), original)
		}
	})

	t.Run("no internal messages returns full copy", func(t *testing.T) {
		t.Parallel()

		ms := message.New(userMsg("a"), assistantMsg("b"))
		got := ms.WithoutInternalMessages()

		assertLen(t, got, 2)
	})

	t.Run("all internal returns empty", func(t *testing.T) {
		t.Parallel()

		ms := message.New(displayOnlyMsg("x"), bookmarkMsg("y"), metadataMsg("z"))
		got := ms.WithoutInternalMessages()

		assertLen(t, got, 0)
	})
}
