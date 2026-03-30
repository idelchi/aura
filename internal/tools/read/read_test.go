package read_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/tools/read"
)

func TestReadFullFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tool := read.New(0, nil, nil)

	got, err := tool.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(got, "1: line1") {
		t.Errorf("output does not contain %q; got:\n%s", "1: line1", got)
	}

	if !strings.Contains(got, "3: line3") {
		t.Errorf("output does not contain %q; got:\n%s", "3: line3", got)
	}
}

func TestReadLineRange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")

	// Build a 100-line file.
	var sb strings.Builder

	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// SmallFileTokens=1 forces line range logic even on small files.
	tool := read.New(1, nil, func(s string) int { return len(s) / 4 })

	got, err := tool.Execute(context.Background(), map[string]any{
		"path":       path,
		"line_start": 5,
		"line_end":   7,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(got, "5: line5") {
		t.Errorf("output does not contain %q; got:\n%s", "5: line5", got)
	}

	if !strings.Contains(got, "7: line7") {
		t.Errorf("output does not contain %q; got:\n%s", "7: line7", got)
	}

	if strings.Contains(got, "4: line4") {
		t.Errorf("output unexpectedly contains line 4 outside requested range; got:\n%s", got)
	}

	if strings.Contains(got, "8: line8") {
		t.Errorf("output unexpectedly contains line 8 outside requested range; got:\n%s", got)
	}
}

func TestReadCount(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "counted.txt")

	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tool := read.New(0, nil, nil)

	got, err := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"count": true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// strings.Split("a\nb\nc\n", "\n") yields ["a","b","c",""] — 4 elements.
	if got != "4" {
		t.Errorf("Execute() count = %q, want %q", got, "4")
	}
}

func TestReadMissingFile(t *testing.T) {
	t.Parallel()

	tool := read.New(0, nil, nil)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": "/nonexistent/path/that/does/not/exist.txt",
	})
	if err == nil {
		t.Errorf("Execute() error = nil, want non-nil for missing file")
	}
}
