package patch_test

import (
	"testing"

	"github.com/idelchi/aura/internal/tools/patch"
)

// assertHunkCount is a helper that fatals if the number of parsed hunks is unexpected.
func assertHunkCount(t *testing.T, hunks []patch.Hunk, want int) {
	t.Helper()

	if len(hunks) != want {
		t.Fatalf("len(hunks) = %d, want %d", len(hunks), want)
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("AddFile hunk", func(t *testing.T) {
		t.Parallel()

		input := "*** Begin Patch\n*** Add File: foo.go\n+package foo\n*** End Patch"

		hunks, err := patch.Parse(input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		assertHunkCount(t, hunks, 1)

		af, ok := hunks[0].(*patch.AddFile)
		if !ok {
			t.Fatalf("hunks[0] type = %T, want *patch.AddFile", hunks[0])
		}

		if af.Path != "foo.go" {
			t.Errorf("AddFile.Path = %q, want %q", af.Path, "foo.go")
		}

		if af.Content != "package foo" {
			t.Errorf("AddFile.Content = %q, want %q", af.Content, "package foo")
		}
	})

	t.Run("DeleteFile hunk", func(t *testing.T) {
		t.Parallel()

		input := "*** Begin Patch\n*** Delete File: old.go\n*** End Patch"

		hunks, err := patch.Parse(input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		assertHunkCount(t, hunks, 1)

		df, ok := hunks[0].(*patch.DeleteFile)
		if !ok {
			t.Fatalf("hunks[0] type = %T, want *patch.DeleteFile", hunks[0])
		}

		if df.Path != "old.go" {
			t.Errorf("DeleteFile.Path = %q, want %q", df.Path, "old.go")
		}
	})

	t.Run("UpdateFile hunk with context and diff lines", func(t *testing.T) {
		t.Parallel()

		input := "*** Begin Patch\n*** Update File: main.go\n@@ func main\n-\tfmt.Println(\"old\")\n+\tfmt.Println(\"new\")\n*** End Patch"

		hunks, err := patch.Parse(input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		assertHunkCount(t, hunks, 1)

		uf, ok := hunks[0].(*patch.UpdateFile)
		if !ok {
			t.Fatalf("hunks[0] type = %T, want *patch.UpdateFile", hunks[0])
		}

		if uf.Path != "main.go" {
			t.Errorf("UpdateFile.Path = %q, want %q", uf.Path, "main.go")
		}

		if len(uf.Chunks) == 0 {
			t.Fatal("UpdateFile.Chunks is empty, want at least one chunk")
		}
	})

	t.Run("multi-hunk patch with Add Update Delete", func(t *testing.T) {
		t.Parallel()

		input := "*** Begin Patch\n" +
			"*** Add File: new.go\n+package new\n" +
			"*** Update File: existing.go\n@@ func foo\n-old\n+new\n" +
			"*** Delete File: gone.go\n" +
			"*** End Patch"

		hunks, err := patch.Parse(input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		assertHunkCount(t, hunks, 3)

		if _, ok := hunks[0].(*patch.AddFile); !ok {
			t.Errorf("hunks[0] type = %T, want *patch.AddFile", hunks[0])
		}

		if _, ok := hunks[1].(*patch.UpdateFile); !ok {
			t.Errorf("hunks[1] type = %T, want *patch.UpdateFile", hunks[1])
		}

		if _, ok := hunks[2].(*patch.DeleteFile); !ok {
			t.Errorf("hunks[2] type = %T, want *patch.DeleteFile", hunks[2])
		}
	})

	t.Run("no Begin Patch marker returns error", func(t *testing.T) {
		t.Parallel()

		input := "*** Add File: foo.go\n+package foo\n*** End Patch"

		_, err := patch.Parse(input)
		if err == nil {
			t.Errorf("Parse() error = nil, want non-nil when Begin Patch is missing")
		}
	})

	t.Run("empty patch markers returns empty slice", func(t *testing.T) {
		t.Parallel()

		input := "*** Begin Patch\n*** End Patch"

		hunks, err := patch.Parse(input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		if len(hunks) != 0 {
			t.Errorf("len(hunks) = %d, want 0", len(hunks))
		}
	})

	t.Run("AddFile with multi-line content", func(t *testing.T) {
		t.Parallel()

		input := "*** Begin Patch\n*** Add File: multi.go\n+package multi\n+\n+func Foo() {}\n*** End Patch"

		hunks, err := patch.Parse(input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		assertHunkCount(t, hunks, 1)

		af, ok := hunks[0].(*patch.AddFile)
		if !ok {
			t.Fatalf("hunks[0] type = %T, want *patch.AddFile", hunks[0])
		}

		if af.Path != "multi.go" {
			t.Errorf("AddFile.Path = %q, want %q", af.Path, "multi.go")
		}
	})

	t.Run("UpdateFile WritePaths returns path", func(t *testing.T) {
		t.Parallel()

		input := "*** Begin Patch\n*** Update File: target.go\n@@ marker\n-old line\n+new line\n*** End Patch"

		hunks, err := patch.Parse(input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		assertHunkCount(t, hunks, 1)

		paths := hunks[0].WritePaths()
		if len(paths) == 0 {
			t.Fatal("WritePaths() = empty, want at least one path")
		}

		if paths[0] != "target.go" {
			t.Errorf("WritePaths()[0] = %q, want %q", paths[0], "target.go")
		}
	})

	t.Run("multiple Begin/End Patch blocks", func(t *testing.T) {
		t.Parallel()

		input := "*** Begin Patch\n*** Add File: a.go\n+package a\n*** End Patch\n" +
			"*** Begin Patch\n*** Delete File: b.go\n*** End Patch"

		hunks, err := patch.Parse(input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		assertHunkCount(t, hunks, 2)
	})
}

func TestSeekLine(t *testing.T) {
	t.Parallel()

	lines := []string{
		"package main",
		"",
		"import \"fmt\"",
		"",
		"func main() {",
		"\tfmt.Println(\"hello\")",
		"}",
	}

	tests := []struct {
		name     string
		lines    []string
		pattern  string
		startIdx int
		want     int
	}{
		{
			name:     "exact match found at first line",
			lines:    lines,
			pattern:  "package main",
			startIdx: 0,
			want:     0,
		},
		{
			name:     "exact match found mid-slice",
			lines:    lines,
			pattern:  "func main()",
			startIdx: 0,
			want:     4,
		},
		{
			name:     "pattern not found returns -1",
			lines:    lines,
			pattern:  "nonexistent pattern",
			startIdx: 0,
			want:     -1,
		},
		{
			name:     "empty pattern returns startIdx",
			lines:    lines,
			pattern:  "",
			startIdx: 0,
			want:     0,
		},
		{
			name:     "empty pattern with non-zero startIdx returns startIdx",
			lines:    lines,
			pattern:  "",
			startIdx: 3,
			want:     3,
		},
		{
			name:     "startIdx skips earlier occurrences",
			lines:    lines,
			pattern:  "\"",
			startIdx: 3, // skip line 2 (import "fmt"), start from line 3
			want:     5, // fmt.Println("hello")
		},
		{
			name:     "partial match within line",
			lines:    lines,
			pattern:  "Println",
			startIdx: 0,
			want:     5,
		},
		{
			name:     "startIdx past end returns -1",
			lines:    lines,
			pattern:  "package",
			startIdx: len(lines),
			want:     -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := patch.SeekLine(tt.lines, tt.pattern, tt.startIdx)
			if got != tt.want {
				t.Errorf("SeekLine(lines, %q, %d) = %d, want %d", tt.pattern, tt.startIdx, got, tt.want)
			}
		})
	}
}

func TestSeekSequence(t *testing.T) {
	t.Parallel()

	lines := []string{
		"func Foo() {",
		"\ta := 1",
		"\tb := 2",
		"\treturn a + b",
		"}",
		"",
		"func Bar() {",
		"\treturn 0",
		"}",
	}

	tests := []struct {
		name      string
		lines     []string
		pattern   []string
		startIdx  int
		wantIdx   int
		wantFound bool
	}{
		{
			name:      "exact sequence found at start",
			lines:     lines,
			pattern:   []string{"func Foo() {", "\ta := 1"},
			startIdx:  0,
			wantIdx:   0,
			wantFound: true,
		},
		{
			name:      "sequence found mid-slice",
			lines:     lines,
			pattern:   []string{"\treturn a + b", "}"},
			startIdx:  0,
			wantIdx:   3,
			wantFound: true,
		},
		{
			name:      "sequence not found returns false",
			lines:     lines,
			pattern:   []string{"nonexistent", "lines here"},
			startIdx:  0,
			wantIdx:   -1,
			wantFound: false,
		},
		{
			name:      "empty pattern returns startIdx true",
			lines:     lines,
			pattern:   []string{},
			startIdx:  0,
			wantIdx:   0,
			wantFound: true,
		},
		{
			name:      "empty pattern with non-zero startIdx",
			lines:     lines,
			pattern:   []string{},
			startIdx:  4,
			wantIdx:   4,
			wantFound: true,
		},
		{
			name:      "startIdx skips earlier match",
			lines:     lines,
			pattern:   []string{"}"},
			startIdx:  5, // skip the first } at index 4
			wantIdx:   8,
			wantFound: true,
		},
		{
			name:      "pattern longer than lines returns false",
			lines:     []string{"a", "b"},
			pattern:   []string{"a", "b", "c"},
			startIdx:  0,
			wantIdx:   -1,
			wantFound: false,
		},
		{
			name:      "single line pattern found",
			lines:     lines,
			pattern:   []string{"func Bar() {"},
			startIdx:  0,
			wantIdx:   6,
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotIdx, gotFound := patch.SeekSequence(tt.lines, tt.pattern, tt.startIdx)

			if gotFound != tt.wantFound {
				t.Errorf("SeekSequence() found = %v, want %v", gotFound, tt.wantFound)
			}

			if gotFound && gotIdx != tt.wantIdx {
				t.Errorf("SeekSequence() idx = %d, want %d", gotIdx, tt.wantIdx)
			}
		})
	}
}
