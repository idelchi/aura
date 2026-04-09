package glob

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// setupTempDir creates a temporary directory with the following layout:
//
//	<tmp>/a.go
//	<tmp>/b.txt
//	<tmp>/sub/c.go
//	<tmp>/sub/deep/d.go
func setupTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	files := []string{
		"a.go",
		"b.txt",
		filepath.Join("sub", "c.go"),
		filepath.Join("sub", "deep", "d.go"),
	}

	for _, rel := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(full), err)
		}

		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", full, err)
		}
	}

	return dir
}

func TestExecuteRecursive(t *testing.T) {
	t.Parallel()

	dir := setupTempDir(t)

	tool := New()

	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "**/*.go",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	if !strings.Contains(out, "a.go") {
		t.Errorf("expected output to contain %q, got:\n%s", "a.go", out)
	}

	if !strings.Contains(out, filepath.Join("sub", "c.go")) {
		t.Errorf("expected output to contain %q, got:\n%s", filepath.Join("sub", "c.go"), out)
	}

	if !strings.Contains(out, filepath.Join("sub", "deep", "d.go")) {
		t.Errorf("expected output to contain %q, got:\n%s", filepath.Join("sub", "deep", "d.go"), out)
	}
}

func TestExecuteTopLevel(t *testing.T) {
	t.Parallel()

	dir := setupTempDir(t)

	tool := New()

	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	if !strings.Contains(out, "b.txt") {
		t.Errorf("expected output to contain %q, got:\n%s", "b.txt", out)
	}

	if strings.Contains(out, "sub") {
		t.Errorf("expected output NOT to contain sub-directory paths, got:\n%s", out)
	}
}

func TestExecuteNoMatch(t *testing.T) {
	t.Parallel()

	dir := setupTempDir(t)

	tool := New()

	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.rs",
		"path":    dir,
	})
	if err != nil {
		t.Errorf("Execute returned unexpected error: %v", err)
	}

	if out != "" {
		t.Errorf("expected empty output for no matches, got: %q", out)
	}
}

func TestExecuteInvalidPattern(t *testing.T) {
	t.Parallel()

	dir := setupTempDir(t)

	tool := New()

	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "[invalid",
		"path":    dir,
	})
	if err == nil {
		t.Errorf("expected an error for invalid pattern, got nil")
	}
}

func TestExecuteSorted(t *testing.T) {
	t.Parallel()

	dir := setupTempDir(t)

	tool := New()

	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "**/*.go",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least a header line and one match, got: %q", out)
	}

	// lines[0] is the "Found N match(es):" header; the rest are paths.
	matchLines := lines[1:]

	sorted := make([]string, len(matchLines))
	copy(sorted, matchLines)
	sort.Strings(sorted)

	for i := range matchLines {
		if matchLines[i] != sorted[i] {
			t.Errorf("output is not sorted: position %d got %q, want %q", i, matchLines[i], sorted[i])
		}
	}
}
