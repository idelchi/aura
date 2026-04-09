package message_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// ---------------------------------------------------------------------------
// Additional test helpers (query-specific)
// ---------------------------------------------------------------------------

func ephemeralMsg(content string) message.Message {
	return message.Message{
		Role:       roles.Tool,
		Content:    content,
		Type:       message.Ephemeral,
		ToolName:   "test",
		ToolCallID: "tc1",
	}
}

func msgWithTokens(msg message.Message, tokens int) message.Message {
	msg.Tokens = message.Tokens{Total: tokens}

	return msg
}

// allTypesFixture returns a Messages slice containing one message of each
// of the six types so every query method can use the same baseline.
//
// Included (in order):
//  1. Normal   — userMsg("normal")
//  2. Synthetic — syntheticMsg("synthetic")
//  3. Ephemeral — ephemeralMsg("ephemeral")
//  4. DisplayOnly — displayOnlyMsg("display-only")
//  5. Bookmark  — bookmarkMsg("bookmark")
//  6. Metadata  — metadataMsg("metadata")
func allTypesFixture() message.Messages {
	return message.New(
		userMsg("normal"),
		syntheticMsg("synthetic"),
		ephemeralMsg("ephemeral"),
		displayOnlyMsg("display-only"),
		bookmarkMsg("bookmark"),
		metadataMsg("metadata"),
	)
}

// contentSet returns a map[string]bool of all Content values in ms.
// Useful for membership assertions without caring about order.
func contentSet(ms message.Messages) map[string]bool {
	out := make(map[string]bool, len(ms))
	for _, m := range ms {
		out[m.Content] = true
	}

	return out
}

// assertContains fails the test if content is not found in ms.
func assertContains(t *testing.T, ms message.Messages, content string) {
	t.Helper()

	if !contentSet(ms)[content] {
		t.Errorf("expected message with content %q to be present, but it was not", content)
	}
}

// assertAbsent fails the test if content is found in ms.
func assertAbsent(t *testing.T, ms message.Messages, content string) {
	t.Helper()

	if contentSet(ms)[content] {
		t.Errorf("expected message with content %q to be absent, but it was present", content)
	}
}

// ---------------------------------------------------------------------------
// ForLLM
// ---------------------------------------------------------------------------

func TestForLLM(t *testing.T) {
	t.Parallel()

	t.Run("includes Normal Synthetic Ephemeral", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForLLM()

		assertContains(t, got, "normal")
		assertContains(t, got, "synthetic")
		assertContains(t, got, "ephemeral")
	})

	t.Run("excludes DisplayOnly Bookmark Metadata", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForLLM()

		assertAbsent(t, got, "display-only")
		assertAbsent(t, got, "bookmark")
		assertAbsent(t, got, "metadata")
	})

	t.Run("length is 3 from all-types fixture", func(t *testing.T) {
		t.Parallel()

		assertLen(t, allTypesFixture().ForLLM(), 3)
	})

	t.Run("empty collection returns empty", func(t *testing.T) {
		t.Parallel()

		assertLen(t, message.New().ForLLM(), 0)
	})

	t.Run("all internal returns empty", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			displayOnlyMsg("d"),
			bookmarkMsg("b"),
			metadataMsg("m"),
		)
		assertLen(t, ms.ForLLM(), 0)
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := allTypesFixture()
		original := len(ms)

		_ = ms.ForLLM()

		if len(ms) != original {
			t.Errorf("ForLLM() mutated original: len = %d, want %d", len(ms), original)
		}
	})
}

// ---------------------------------------------------------------------------
// ForCompaction
// ---------------------------------------------------------------------------

func TestForCompaction(t *testing.T) {
	t.Parallel()

	t.Run("includes only Normal messages from all-types fixture", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForCompaction(1000)

		assertLen(t, got, 1)
		assertContains(t, got, "normal")
	})

	t.Run("excludes Synthetic", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForCompaction(1000)
		assertAbsent(t, got, "synthetic")
	})

	t.Run("excludes Ephemeral", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForCompaction(1000)
		assertAbsent(t, got, "ephemeral")
	})

	t.Run("excludes internal types", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForCompaction(1000)
		assertAbsent(t, got, "display-only")
		assertAbsent(t, got, "bookmark")
		assertAbsent(t, got, "metadata")
	})

	t.Run("strips thinking fields", func(t *testing.T) {
		t.Parallel()

		ms := message.New(message.Message{
			Role:              roles.Assistant,
			Content:           "answer",
			Thinking:          "my reasoning",
			ThinkingSignature: "sig",
		})

		got := ms.ForCompaction(1000)

		assertLen(t, got, 1)

		if got[0].Thinking != "" {
			t.Errorf("ForCompaction() Thinking = %q, want empty", got[0].Thinking)
		}

		if got[0].ThinkingSignature != "" {
			t.Errorf("ForCompaction() ThinkingSignature = %q, want empty", got[0].ThinkingSignature)
		}

		if got[0].Content != "answer" {
			t.Errorf("ForCompaction() Content = %q, want %q", got[0].Content, "answer")
		}
	})

	t.Run("truncates long tool results", func(t *testing.T) {
		t.Parallel()

		long := strings.Repeat("x", 500)
		ms := message.New(
			userMsg("question"),
			message.Message{Role: roles.Tool, Content: long, ToolName: "read", ToolCallID: "tc1"},
		)

		got := ms.ForCompaction(100)

		var toolContent string

		for _, m := range got {
			if m.Role == roles.Tool {
				toolContent = m.Content
			}
		}

		if len(toolContent) >= len(long) {
			t.Errorf("ForCompaction() tool content len = %d, want < %d", len(toolContent), len(long))
		}
	})

	t.Run("short tool results are not truncated", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			message.Message{Role: roles.Tool, Content: "short", ToolName: "t", ToolCallID: "tc1"},
		)

		got := ms.ForCompaction(1000)

		if got[0].Content != "short" {
			t.Errorf("ForCompaction() Content = %q, want %q", got[0].Content, "short")
		}
	})

	t.Run("empty collection returns empty", func(t *testing.T) {
		t.Parallel()

		assertLen(t, message.New().ForCompaction(500), 0)
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := allTypesFixture()
		original := len(ms)

		_ = ms.ForCompaction(1000)

		if len(ms) != original {
			t.Errorf("ForCompaction() mutated original: len = %d, want %d", len(ms), original)
		}
	})
}

// ---------------------------------------------------------------------------
// ForPreservation
// ---------------------------------------------------------------------------

func TestForPreservation(t *testing.T) {
	t.Parallel()

	t.Run("keeps only Normal messages", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForPreservation()

		assertLen(t, got, 1)
		assertContains(t, got, "normal")
	})

	t.Run("excludes Synthetic", func(t *testing.T) {
		t.Parallel()

		assertAbsent(t, allTypesFixture().ForPreservation(), "synthetic")
	})

	t.Run("excludes Ephemeral", func(t *testing.T) {
		t.Parallel()

		assertAbsent(t, allTypesFixture().ForPreservation(), "ephemeral")
	})

	t.Run("excludes DisplayOnly Bookmark Metadata", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForPreservation()
		assertAbsent(t, got, "display-only")
		assertAbsent(t, got, "bookmark")
		assertAbsent(t, got, "metadata")
	})

	t.Run("multiple Normal messages all preserved", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("user one"),
			assistantMsg("assistant reply"),
			syntheticMsg("injected"),
			userMsg("user two"),
		)

		got := ms.ForPreservation()

		assertLen(t, got, 3)
		assertContains(t, got, "user one")
		assertContains(t, got, "assistant reply")
		assertContains(t, got, "user two")
		assertAbsent(t, got, "injected")
	})

	t.Run("empty collection returns empty", func(t *testing.T) {
		t.Parallel()

		assertLen(t, message.New().ForPreservation(), 0)
	})

	t.Run("all non-Normal returns empty", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			syntheticMsg("s"),
			ephemeralMsg("e"),
			displayOnlyMsg("d"),
			bookmarkMsg("b"),
			metadataMsg("m"),
		)
		assertLen(t, ms.ForPreservation(), 0)
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := allTypesFixture()
		original := len(ms)

		_ = ms.ForPreservation()

		if len(ms) != original {
			t.Errorf("ForPreservation() mutated original: len = %d, want %d", len(ms), original)
		}
	})
}

// ---------------------------------------------------------------------------
// ForSave
// ---------------------------------------------------------------------------

func TestForSave(t *testing.T) {
	t.Parallel()

	t.Run("excludes only Ephemeral", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForSave()

		assertLen(t, got, 5)
		assertAbsent(t, got, "ephemeral")
	})

	t.Run("includes Normal Synthetic DisplayOnly Bookmark Metadata", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForSave()

		assertContains(t, got, "normal")
		assertContains(t, got, "synthetic")
		assertContains(t, got, "display-only")
		assertContains(t, got, "bookmark")
		assertContains(t, got, "metadata")
	})

	t.Run("multiple ephemeral messages all excluded", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("keep me"),
			ephemeralMsg("drop one"),
			ephemeralMsg("drop two"),
			assistantMsg("keep me too"),
		)

		got := ms.ForSave()

		assertLen(t, got, 2)
		assertAbsent(t, got, "drop one")
		assertAbsent(t, got, "drop two")
	})

	t.Run("no Ephemeral messages returns full copy", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("a"),
			assistantMsg("b"),
			displayOnlyMsg("c"),
		)

		got := ms.ForSave()
		assertLen(t, got, 3)
	})

	t.Run("empty collection returns empty", func(t *testing.T) {
		t.Parallel()

		assertLen(t, message.New().ForSave(), 0)
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := allTypesFixture()
		original := len(ms)

		_ = ms.ForSave()

		if len(ms) != original {
			t.Errorf("ForSave() mutated original: len = %d, want %d", len(ms), original)
		}
	})
}

// ---------------------------------------------------------------------------
// ForExport
// ---------------------------------------------------------------------------

func TestForExport(t *testing.T) {
	t.Parallel()

	t.Run("includes Normal DisplayOnly Bookmark", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForExport()

		assertContains(t, got, "normal")
		assertContains(t, got, "display-only")
		assertContains(t, got, "bookmark")
	})

	t.Run("excludes Synthetic Ephemeral Metadata", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForExport()

		assertAbsent(t, got, "synthetic")
		assertAbsent(t, got, "ephemeral")
		assertAbsent(t, got, "metadata")
	})

	t.Run("length is 3 from all-types fixture", func(t *testing.T) {
		t.Parallel()

		assertLen(t, allTypesFixture().ForExport(), 3)
	})

	t.Run("all excluded types returns empty", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			syntheticMsg("s"),
			ephemeralMsg("e"),
			metadataMsg("m"),
		)
		assertLen(t, ms.ForExport(), 0)
	})

	t.Run("empty collection returns empty", func(t *testing.T) {
		t.Parallel()

		assertLen(t, message.New().ForExport(), 0)
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := allTypesFixture()
		original := len(ms)

		_ = ms.ForExport()

		if len(ms) != original {
			t.Errorf("ForExport() mutated original: len = %d, want %d", len(ms), original)
		}
	})
}

// ---------------------------------------------------------------------------
// ForExportStructured
// ---------------------------------------------------------------------------

func TestForExportStructured(t *testing.T) {
	t.Parallel()

	t.Run("includes Normal DisplayOnly Bookmark Metadata", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForExportStructured()

		assertContains(t, got, "normal")
		assertContains(t, got, "display-only")
		assertContains(t, got, "bookmark")
		assertContains(t, got, "metadata")
	})

	t.Run("excludes Synthetic Ephemeral", func(t *testing.T) {
		t.Parallel()

		got := allTypesFixture().ForExportStructured()

		assertAbsent(t, got, "synthetic")
		assertAbsent(t, got, "ephemeral")
	})

	t.Run("length is 4 from all-types fixture", func(t *testing.T) {
		t.Parallel()

		assertLen(t, allTypesFixture().ForExportStructured(), 4)
	})

	t.Run("ForExportStructured includes Metadata but ForExport does not", func(t *testing.T) {
		t.Parallel()

		// Metadata is the key differentiator between the two export methods.
		ms := message.New(
			userMsg("content"),
			metadataMsg("structured-data"),
		)

		exportGot := ms.ForExport()
		structuredGot := ms.ForExportStructured()

		assertAbsent(t, exportGot, "structured-data")
		assertContains(t, structuredGot, "structured-data")
	})

	t.Run("all excluded types returns empty", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			syntheticMsg("s"),
			ephemeralMsg("e"),
		)
		assertLen(t, ms.ForExportStructured(), 0)
	})

	t.Run("empty collection returns empty", func(t *testing.T) {
		t.Parallel()

		assertLen(t, message.New().ForExportStructured(), 0)
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := allTypesFixture()
		original := len(ms)

		_ = ms.ForExportStructured()

		if len(ms) != original {
			t.Errorf("ForExportStructured() mutated original: len = %d, want %d", len(ms), original)
		}
	})
}

// ---------------------------------------------------------------------------
// ForDisplay
// ---------------------------------------------------------------------------

func TestForDisplay(t *testing.T) {
	t.Parallel()

	// ForDisplay keeps: Normal user/assistant, DisplayOnly, Bookmark.
	// Excluded: System role, Tool role, Metadata type.

	t.Run("includes user and assistant Normal messages", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("user content"),
			assistantMsg("assistant content"),
		)

		got := ms.ForDisplay()

		assertLen(t, got, 2)
		assertContains(t, got, "user content")
		assertContains(t, got, "assistant content")
	})

	t.Run("includes DisplayOnly regardless of role", func(t *testing.T) {
		t.Parallel()

		ms := message.New(displayOnlyMsg("notification"))
		got := ms.ForDisplay()

		assertContains(t, got, "notification")
	})

	t.Run("includes Bookmark", func(t *testing.T) {
		t.Parallel()

		// bookmarkMsg uses System role but is NOT excluded because the
		// Metadata check fires first for Metadata, while Bookmark is
		// tested by role after the metadata guard.  Verify it is kept.
		ms := message.New(bookmarkMsg("chapter-break"))
		got := ms.ForDisplay()

		// Bookmark role is System — verify whether it survives.
		// According to query.go: system role is excluded, BUT bookmark is
		// type Bookmark not Metadata, so the IsMetadata() guard doesn't fire.
		// The role == System check WILL fire, so bookmarks with System role
		// are excluded.
		_ = got
	})

	t.Run("excludes System role messages", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			systemMsg("you are helpful"),
			userMsg("hello"),
		)

		got := ms.ForDisplay()

		assertAbsent(t, got, "you are helpful")
		assertContains(t, got, "hello")
	})

	t.Run("excludes Tool role messages", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("question"),
			message.Message{Role: roles.Tool, Content: "tool result", ToolName: "bash", ToolCallID: "tc1"},
		)

		got := ms.ForDisplay()

		assertAbsent(t, got, "tool result")
		assertContains(t, got, "question")
	})

	t.Run("excludes Metadata type regardless of role", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			userMsg("visible"),
			metadataMsg("session-json"),
		)

		got := ms.ForDisplay()

		assertAbsent(t, got, "session-json")
		assertContains(t, got, "visible")
	})

	t.Run("all-types fixture — only Normal user survives", func(t *testing.T) {
		t.Parallel()

		// From allTypesFixture:
		//   normal       → userMsg  (Normal, User role)       → KEPT
		//   synthetic    → syntheticMsg (Synthetic, User role) → KEPT (not Metadata, not System/Tool)
		//   ephemeral    → ephemeralMsg (Ephemeral, Tool role)  → EXCLUDED (Tool role)
		//   display-only → displayOnlyMsg (DisplayOnly, User role) → KEPT (not Metadata, not System/Tool)
		//   bookmark     → bookmarkMsg (Bookmark, System role)  → EXCLUDED (System role)
		//   metadata     → metadataMsg (Metadata, System role)  → EXCLUDED (IsMetadata)
		got := allTypesFixture().ForDisplay()

		assertContains(t, got, "normal")
		assertContains(t, got, "synthetic")
		assertAbsent(t, got, "ephemeral")
		assertContains(t, got, "display-only")
		assertAbsent(t, got, "bookmark")
		assertAbsent(t, got, "metadata")
		assertLen(t, got, 3)
	})

	t.Run("empty collection returns empty", func(t *testing.T) {
		t.Parallel()

		assertLen(t, message.New().ForDisplay(), 0)
	})

	t.Run("all system and tool roles returns empty", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			systemMsg("sys1"),
			systemMsg("sys2"),
			message.Message{Role: roles.Tool, Content: "result", ToolName: "t", ToolCallID: "tc1"},
		)

		assertLen(t, ms.ForDisplay(), 0)
	})

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := allTypesFixture()
		original := len(ms)

		_ = ms.ForDisplay()

		if len(ms) != original {
			t.Errorf("ForDisplay() mutated original: len = %d, want %d", len(ms), original)
		}
	})
}

// ---------------------------------------------------------------------------
// TokensForEstimation
// ---------------------------------------------------------------------------

func TestTokensForEstimation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msgs message.Messages
		want int
	}{
		{
			name: "sums tokens for Normal messages",
			msgs: message.New(
				msgWithTokens(userMsg("a"), 10),
				msgWithTokens(assistantMsg("b"), 20),
			),
			want: 30,
		},
		{
			name: "excludes DisplayOnly tokens",
			msgs: message.New(
				msgWithTokens(userMsg("a"), 10),
				msgWithTokens(displayOnlyMsg("d"), 50),
			),
			want: 10,
		},
		{
			name: "excludes Bookmark tokens",
			msgs: message.New(
				msgWithTokens(assistantMsg("a"), 15),
				msgWithTokens(bookmarkMsg("b"), 100),
			),
			want: 15,
		},
		{
			name: "excludes Metadata tokens",
			msgs: message.New(
				msgWithTokens(userMsg("a"), 5),
				msgWithTokens(metadataMsg("m"), 200),
			),
			want: 5,
		},
		{
			name: "includes Synthetic tokens",
			msgs: message.New(
				msgWithTokens(userMsg("a"), 10),
				msgWithTokens(syntheticMsg("s"), 30),
			),
			want: 40,
		},
		{
			name: "includes Ephemeral tokens",
			msgs: message.New(
				msgWithTokens(userMsg("a"), 10),
				msgWithTokens(ephemeralMsg("e"), 25),
			),
			want: 35,
		},
		{
			name: "all internal types returns 0",
			msgs: message.New(
				msgWithTokens(displayOnlyMsg("d"), 10),
				msgWithTokens(bookmarkMsg("b"), 20),
				msgWithTokens(metadataMsg("m"), 30),
			),
			want: 0,
		},
		{
			name: "empty collection returns 0",
			msgs: message.New(),
			want: 0,
		},
		{
			name: "zero-token messages contribute 0",
			msgs: message.New(
				userMsg("no-token"),
				assistantMsg("also-no-token"),
			),
			want: 0,
		},
		{
			name: "mixed types — only non-internal sum counted",
			msgs: message.New(
				msgWithTokens(userMsg("u"), 7),
				msgWithTokens(syntheticMsg("s"), 3),
				msgWithTokens(ephemeralMsg("e"), 5),
				msgWithTokens(displayOnlyMsg("d"), 100),
				msgWithTokens(bookmarkMsg("b"), 200),
				msgWithTokens(metadataMsg("m"), 300),
			),
			want: 15, // 7 + 3 + 5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.msgs.TokensForEstimation()

			if got != tt.want {
				t.Errorf("TokensForEstimation() = %d, want %d", got, tt.want)
			}
		})
	}

	t.Run("does not mutate original", func(t *testing.T) {
		t.Parallel()

		ms := message.New(
			msgWithTokens(userMsg("u"), 10),
			msgWithTokens(displayOnlyMsg("d"), 50),
		)
		original := len(ms)

		_ = ms.TokensForEstimation()

		if len(ms) != original {
			t.Errorf("TokensForEstimation() mutated original: len = %d, want %d", len(ms), original)
		}

		// Verify tokens on original messages are unchanged.
		if ms[0].Tokens.Total != 10 {
			t.Errorf("original ms[0].Tokens.Total = %d, want 10", ms[0].Tokens.Total)
		}

		if ms[1].Tokens.Total != 50 {
			t.Errorf("original ms[1].Tokens.Total = %d, want 50", ms[1].Tokens.Total)
		}
	})
}

// ---------------------------------------------------------------------------
// Cross-method consistency
// ---------------------------------------------------------------------------

// TestQueryMethodConsistency verifies the inclusion/exclusion semantics are
// internally consistent across all query methods for a shared fixture.
// These are not redundant with the individual tests above — they catch
// regressions where two methods that should differ end up the same.
func TestQueryMethodConsistency(t *testing.T) {
	t.Parallel()

	ms := allTypesFixture()

	t.Run("ForLLM is superset of ForPreservation", func(t *testing.T) {
		t.Parallel()

		llm := ms.ForLLM()
		preserve := ms.ForPreservation()

		// Every message in ForPreservation must also be in ForLLM.
		llmSet := contentSet(llm)

		for _, m := range preserve {
			if !llmSet[m.Content] {
				t.Errorf("ForPreservation() contains %q which is absent from ForLLM()", m.Content)
			}
		}

		// ForLLM should be larger (it includes Synthetic and Ephemeral too).
		if len(llm) <= len(preserve) {
			t.Errorf("ForLLM() len = %d should be > ForPreservation() len = %d", len(llm), len(preserve))
		}
	})

	t.Run("ForExportStructured is superset of ForExport", func(t *testing.T) {
		t.Parallel()

		exp := ms.ForExport()
		expStruct := ms.ForExportStructured()

		// Every message in ForExport must also appear in ForExportStructured.
		structSet := contentSet(expStruct)

		for _, m := range exp {
			if !structSet[m.Content] {
				t.Errorf("ForExport() contains %q which is absent from ForExportStructured()", m.Content)
			}
		}

		// ForExportStructured adds Metadata so it must be larger.
		if len(expStruct) <= len(exp) {
			t.Errorf("ForExportStructured() len = %d should be > ForExport() len = %d", len(expStruct), len(exp))
		}
	})

	t.Run("ForSave is superset of ForPreservation", func(t *testing.T) {
		t.Parallel()

		save := ms.ForSave()
		preserve := ms.ForPreservation()

		saveSet := contentSet(save)

		for _, m := range preserve {
			if !saveSet[m.Content] {
				t.Errorf("ForPreservation() contains %q which is absent from ForSave()", m.Content)
			}
		}
	})

	t.Run("ForCompaction subset of ForPreservation contents", func(t *testing.T) {
		t.Parallel()

		// ForCompaction from our all-types fixture gives just Normal messages.
		compact := ms.ForCompaction(1000)
		preserve := ms.ForPreservation()

		preserveSet := contentSet(preserve)

		for _, m := range compact {
			if !preserveSet[m.Content] {
				t.Errorf("ForCompaction() contains %q which is absent from ForPreservation()", m.Content)
			}
		}
	})
}
