package ls_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/tools/ls"
)

// mustMkdir creates a directory hierarchy and fatals on error.
func mustMkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

// mustWriteFile creates a file with empty content and fatals on error.
func mustWriteFile(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func TestLsDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "sub"))
	mustWriteFile(t, filepath.Join(dir, "a.txt"))
	mustWriteFile(t, filepath.Join(dir, "b.go"))

	tool := ls.New()

	got, err := tool.Execute(context.Background(), map[string]any{"path": dir})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lines := strings.Split(got, "\n")

	// First entry must be the directory (with trailing slash).
	if len(lines) == 0 || !strings.HasSuffix(lines[0], "/") {
		t.Errorf("first output line = %q, want a directory entry ending with '/'", lines[0])
	}

	// Verify both files are present.
	if !strings.Contains(got, "a.txt") {
		t.Errorf("output does not contain %q; got:\n%s", "a.txt", got)
	}

	if !strings.Contains(got, "b.go") {
		t.Errorf("output does not contain %q; got:\n%s", "b.go", got)
	}

	// Files must appear after directories: find index of sub/ and a.txt.
	subIdx := -1
	aTxtIdx := -1

	for i, line := range lines {
		if line == "sub/" {
			subIdx = i
		}

		if line == "a.txt" {
			aTxtIdx = i
		}
	}

	if subIdx == -1 {
		t.Fatalf("output does not contain %q", "sub/")
	}

	if aTxtIdx == -1 {
		t.Fatalf("output does not contain %q", "a.txt")
	}

	if subIdx >= aTxtIdx {
		t.Errorf("directory sub/ (line %d) should appear before file a.txt (line %d)", subIdx, aTxtIdx)
	}
}

func TestLsDepth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "sub", "nested"))
	mustWriteFile(t, filepath.Join(dir, "sub", "nested", "file.txt"))

	tool := ls.New()

	t.Run("depth 1 sees only top-level sub dir", func(t *testing.T) {
		t.Parallel()

		got, err := tool.Execute(context.Background(), map[string]any{
			"path":  dir,
			"depth": 1,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !strings.Contains(got, "sub/") {
			t.Errorf("output does not contain %q; got:\n%s", "sub/", got)
		}

		if strings.Contains(got, "nested") {
			t.Errorf("output unexpectedly contains %q at depth 1; got:\n%s", "nested", got)
		}
	})

	t.Run("depth 2 sees sub and nested", func(t *testing.T) {
		t.Parallel()

		got, err := tool.Execute(context.Background(), map[string]any{
			"path":  dir,
			"depth": 2,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !strings.Contains(got, "sub/") {
			t.Errorf("output does not contain %q; got:\n%s", "sub/", got)
		}

		if !strings.Contains(got, "nested") {
			t.Errorf("output does not contain %q at depth 2; got:\n%s", "nested", got)
		}
	})
}

func TestLsEmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	tool := ls.New()

	got, err := tool.Execute(context.Background(), map[string]any{"path": dir})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got != "Empty directory" {
		t.Errorf("Execute() = %q, want %q", got, "Empty directory")
	}
}

func TestLsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	mustWriteFile(t, path)

	tool := ls.New()

	got, err := tool.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got != "target.txt" {
		t.Errorf("Execute() = %q, want %q", got, "target.txt")
	}
}
