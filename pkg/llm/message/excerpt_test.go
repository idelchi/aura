package message_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// excerptMarker matches the marker ExcerptResults appends to a cut result.
var excerptMarker = regexp.MustCompile(
	`\n\[tool result excerpt: first (\d+) of (\d+) bytes shown; the remaining (\d+) bytes are absent from this transcript\]$`)

// omissionMarker matches the content ExcerptResults leaves for an omitted result.
var omissionMarker = regexp.MustCompile(`^\[tool result omitted: all (\d+) bytes are absent from this transcript\]$`)

// excerptOf splits an excerpted result into the shown text and the marker's counts.
func excerptOf(t *testing.T, content string) (shown string, first, total, remaining int) {
	t.Helper()

	loc := excerptMarker.FindStringSubmatchIndex(content)
	if loc == nil {
		t.Fatalf("no excerpt marker in %q", content)
	}

	count := func(group int) int {
		n, err := strconv.Atoi(content[loc[2*group]:loc[2*group+1]])
		if err != nil {
			t.Fatal(err)
		}

		return n
	}

	return content[:loc[0]], count(1), count(2), count(3)
}

func TestExcerptResults(t *testing.T) {
	t.Parallel()

	t.Run("results within the limit and other roles are unchanged", func(t *testing.T) {
		t.Parallel()

		long := strings.Repeat("user text ", 100)
		ms := message.New(userMsg(long), assistantMsg(long), toolMsg("read", "exact"), toolMsg("read", "short"), toolMsg("read", ""))

		got := ms.ExcerptResults(5)

		for i := range ms {
			if got[i].Content != ms[i].Content {
				t.Errorf("message %d changed: %q", i, got[i].Content)
			}
		}
	})

	t.Run("a cut result states what was shown and what is absent", func(t *testing.T) {
		t.Parallel()

		original := `{"ok": true, "result": {"stance": [2.5, 1.2], "attacks": ["Probing jab", "Heavy swing"]}}`
		got := message.New(toolMsg("inspect", original)).ExcerptResults(40)[0].Content

		shown, first, total, remaining := excerptOf(t, got)
		if shown != original[:40] || first != 40 || total != len(original) || remaining != len(original)-40 {
			t.Fatalf("excerpt %q first=%d total=%d remaining=%d", shown, first, total, remaining)
		}

		if strings.Contains(got, "Heavy swing") || strings.HasSuffix(shown, "...") {
			t.Fatalf("excerpt carries omitted text or an unexplained ellipsis: %q", got)
		}
	})

	t.Run("a cut never splits a character and counts the bytes actually shown", func(t *testing.T) {
		t.Parallel()

		original := strings.Repeat("Wenna’s socket 😀 ", 20)
		for limit := 1; limit < 64; limit++ {
			got := message.New(toolMsg("read", original)).ExcerptResults(limit)[0].Content
			if omissionMarker.MatchString(got) {
				continue
			}

			shown, first, total, remaining := excerptOf(t, got)
			if !utf8.ValidString(shown) || !strings.HasPrefix(original, shown) || len(shown) > limit ||
				first != len(shown) || total != len(original) || first+remaining != total {
				t.Fatalf("limit %d: shown=%q first=%d total=%d remaining=%d", limit, shown, first, total, remaining)
			}
		}
	})

	t.Run("zero and negative limits omit results with their size", func(t *testing.T) {
		t.Parallel()

		original := strings.Repeat("evidence ", 50)
		for _, limit := range []int{0, -1} {
			got := message.New(toolMsg("read", original), toolMsg("read", "")).ExcerptResults(limit)

			if m := omissionMarker.FindStringSubmatch(got[0].Content); m == nil || m[1] != fmt.Sprint(len(original)) {
				t.Fatalf("limit %d: %q", limit, got[0].Content)
			}

			if got[1].Content != "" {
				t.Fatalf("limit %d: an empty result became %q", limit, got[1].Content)
			}
		}
	})

	t.Run("a limit below the first character omits instead of showing nothing", func(t *testing.T) {
		t.Parallel()

		got := message.New(toolMsg("read", "😀 rest")).ExcerptResults(2)[0].Content
		if !omissionMarker.MatchString(got) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("the input is not mutated", func(t *testing.T) {
		t.Parallel()

		ms := message.New(toolMsg("read", strings.Repeat("x", 100)))
		_ = ms.ExcerptResults(10)

		if len(ms[0].Content) != 100 {
			t.Fatal("ExcerptResults mutated its receiver")
		}
	})

	t.Run("ResultsBeyond counts what would be cut or omitted", func(t *testing.T) {
		t.Parallel()

		ms := message.New(userMsg(strings.Repeat("u", 50)), toolMsg("a", strings.Repeat("x", 50)), toolMsg("b", "tiny"), toolMsg("c", ""))
		if got := ms.ResultsBeyond(10); got != 1 {
			t.Fatalf("ResultsBeyond(10) = %d, want 1", got)
		}

		if got := ms.ResultsBeyond(0); got != 2 {
			t.Fatalf("ResultsBeyond(0) = %d, want 2 non-empty results", got)
		}
	})
}

// TestForCompactionMarksEvidence: the compaction view marks cuts, while the generic
// Truncated view keeps its silent "..." semantics and ignores non-positive limits.
func TestForCompactionMarksEvidence(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("y", 300)
	ms := message.New(
		userMsg("question"),
		message.Message{Role: roles.Assistant, Content: "answer", Thinking: "private"},
		toolMsg("read", long),
	)

	compacted := ms.ForCompaction(100)
	if compacted[1].Thinking != "" {
		t.Fatal("thinking reached the compaction view")
	}

	if _, first, total, _ := excerptOf(t, compacted[2].Content); first != 100 || total != 300 {
		t.Fatalf("first=%d total=%d", first, total)
	}

	if !omissionMarker.MatchString(ms.ForCompaction(0)[2].Content) {
		t.Fatalf("ForCompaction(0) = %q", ms.ForCompaction(0)[2].Content)
	}

	generic := ms.Truncated(100)[2].Content
	if !strings.HasSuffix(generic, "...") || len(generic) > 100 || strings.Contains(generic, "absent") {
		t.Fatalf("Truncated changed semantics: %q", generic)
	}

	if ms.Truncated(0)[2].Content != long {
		t.Fatal("Truncated(0) must leave results unchanged")
	}
}
