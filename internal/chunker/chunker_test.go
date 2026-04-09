package chunker_test

import (
	"context"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/chunker"
)

// generateLines builds a string with n lines each of the given length.
// Each line is repeated chars so token estimation (len/4) is predictable.
func generateLines(n, charsPerLine int) string {
	line := strings.Repeat("x", charsPerLine)

	lines := make([]string, n)
	for i := range n {
		lines[i] = line
	}

	return strings.Join(lines, "\n")
}

func TestChunkSmallText(t *testing.T) {
	t.Parallel()

	// MaxTokens=50 means content up to 200 chars fits in a single chunk (estimate = len/4).
	// "hello world" is 11 chars → 1 token → well within 50.
	cfg := chunker.Config{
		Strategy:  chunker.StrategyLine,
		MaxTokens: 50,
		Estimate: func(s string) int {
			if len(s) == 0 {
				return 0
			}

			return max(len(s)/4, 1)
		},
	}

	c := chunker.New(cfg)
	content := "hello world"
	chunks := c.Chunk(context.Background(), "file.txt", content)

	if len(chunks) != 1 {
		t.Errorf("len(chunks) = %d, want 1 for small text", len(chunks))
	}

	if len(chunks) > 0 {
		if chunks[0].Index != 0 {
			t.Errorf("chunks[0].Index = %d, want 0", chunks[0].Index)
		}

		if chunks[0].Content != content {
			t.Errorf("chunks[0].Content = %q, want %q", chunks[0].Content, content)
		}

		if chunks[0].StartLine != 1 {
			t.Errorf("chunks[0].StartLine = %d, want 1", chunks[0].StartLine)
		}
	}
}

func TestChunkLargeText(t *testing.T) {
	t.Parallel()

	// MaxTokens=50 → 200 chars per chunk max.
	// Generate 30 lines of 40 chars each → 40 tokens per line, exceeds limit after 1 line.
	// This forces multiple chunks.
	cfg := chunker.Config{
		Strategy:      chunker.StrategyLine,
		MaxTokens:     50,
		OverlapTokens: 0,
		Estimate: func(s string) int {
			if len(s) == 0 {
				return 0
			}

			return max(len(s)/4, 1)
		},
	}

	c := chunker.New(cfg)
	// 20 lines × 40 chars = 800 chars total, ~200 tokens — well above MaxTokens=50
	content := generateLines(20, 40)
	chunks := c.Chunk(context.Background(), "file.txt", content)

	if len(chunks) < 2 {
		t.Errorf("len(chunks) = %d, want >= 2 for large text", len(chunks))
	}

	for i, ch := range chunks {
		if ch.Content == "" {
			t.Errorf("chunks[%d].Content is empty", i)
		}

		if ch.Index != i {
			t.Errorf("chunks[%d].Index = %d, want %d", i, ch.Index, i)
		}

		if ch.StartLine < 1 {
			t.Errorf("chunks[%d].StartLine = %d, want >= 1", i, ch.StartLine)
		}

		if ch.EndLine < ch.StartLine {
			t.Errorf("chunks[%d].EndLine=%d < StartLine=%d", i, ch.EndLine, ch.StartLine)
		}
	}
}

func TestChunkEmpty(t *testing.T) {
	t.Parallel()

	cfg := chunker.Config{
		Strategy:  chunker.StrategyLine,
		MaxTokens: 50,
		Estimate: func(s string) int {
			if len(s) == 0 {
				return 0
			}

			return max(len(s)/4, 1)
		},
	}

	c := chunker.New(cfg)
	chunks := c.Chunk(context.Background(), "file.txt", "")

	// Empty content: estimate("") == 0, which is <= MaxTokens, so a single chunk is returned.
	// That single chunk has empty Content and EndLine=0 (countLines("") == 0).
	if len(chunks) != 1 {
		t.Errorf("len(chunks) = %d, want 1 for empty input", len(chunks))
	}
}

func TestChunksContents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		chunks chunker.Chunks
		want   []string
	}{
		{
			name:   "empty chunks",
			chunks: chunker.Chunks{},
			want:   []string{},
		},
		{
			name: "single chunk",
			chunks: chunker.Chunks{
				{Index: 0, Content: "hello", StartLine: 1, EndLine: 1},
			},
			want: []string{"hello"},
		},
		{
			name: "multiple chunks",
			chunks: chunker.Chunks{
				{Index: 0, Content: "first", StartLine: 1, EndLine: 2},
				{Index: 1, Content: "second", StartLine: 3, EndLine: 4},
				{Index: 2, Content: "third", StartLine: 5, EndLine: 6},
			},
			want: []string{"first", "second", "third"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.chunks.Contents()

			if len(got) != len(tt.want) {
				t.Errorf("Contents() len = %d, want %d", len(got), len(tt.want))

				return
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("Contents()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
