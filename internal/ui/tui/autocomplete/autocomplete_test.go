package autocomplete

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompleteNoAt(t *testing.T) {
	t.Parallel()

	c := New("/tmp")

	got := c.Complete("hello world", 11)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestCompleteDirectiveName(t *testing.T) {
	t.Parallel()

	c := New("/tmp")

	// Just "@" typed — expects first directive suggestion.
	got := c.Complete("@", 1)
	if got != "File[" {
		t.Errorf("Complete(\"@\", 1): expected %q, got %q", "File[", got)
	}

	// Partial directive name.
	got = c.Complete("@Fi", 3)
	if got != "le[" {
		t.Errorf("Complete(\"@Fi\", 3): expected %q, got %q", "le[", got)
	}
}

func TestCompleteDirectiveExact(t *testing.T) {
	t.Parallel()

	c := New("/tmp")

	got := c.Complete("@File", 5)
	if got != "[" {
		t.Errorf("Complete(\"@File\", 5): expected %q, got %q", "[", got)
	}
}

func TestCompleteClosedDirective(t *testing.T) {
	t.Parallel()

	c := New("/tmp")
	text := "@File[path]"

	got := c.Complete(text, len(text))
	if got != "" {
		t.Errorf("expected empty string for closed directive, got %q", got)
	}
}

func TestCompleteFilePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "src", "utils"), 0o755); err != nil {
		t.Fatalf("MkdirAll src/utils: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile README.md: %v", err)
	}

	c := New(dir)
	text := "@File[src/m"

	got := c.Complete(text, len(text))
	if got != "ain.go]" {
		t.Errorf("expected %q, got %q", "ain.go]", got)
	}
}

func TestCompleteDirectoryPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "src", "utils"), 0o755); err != nil {
		t.Fatalf("MkdirAll src/utils: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile README.md: %v", err)
	}

	c := New(dir)
	text := "@File[sr"

	got := c.Complete(text, len(text))
	if got != "c/" {
		t.Errorf("expected %q, got %q", "c/", got)
	}
}

func TestCompleteNoMatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "src", "utils"), 0o755); err != nil {
		t.Fatalf("MkdirAll src/utils: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile README.md: %v", err)
	}

	c := New(dir)
	text := "@File[nonexistent"

	got := c.Complete(text, len(text))
	if got != "" {
		t.Errorf("expected empty string for nonexistent path, got %q", got)
	}
}

func TestCompleteBashNoPath(t *testing.T) {
	t.Parallel()

	c := New("/tmp")
	text := "@Bash["

	got := c.Complete(text, len(text))
	if got != "" {
		t.Errorf("expected empty string for @Bash[ (no path completion), got %q", got)
	}
}

func TestAcceptInsertsHint(t *testing.T) {
	t.Parallel()

	c := New("/tmp")

	newText, newCursor, ok := c.Accept("@", 1)
	if !ok {
		t.Fatalf("Accept(\"@\", 1): expected ok=true, got false")
	}

	if newText != "@File[" {
		t.Errorf("Accept(\"@\", 1): expected newText=%q, got %q", "@File[", newText)
	}

	if newCursor != 6 {
		t.Errorf("Accept(\"@\", 1): expected newCursor=6, got %d", newCursor)
	}
}

func TestAcceptNoCompletion(t *testing.T) {
	t.Parallel()

	c := New("/tmp")

	newText, newCursor, ok := c.Accept("hello", 5)
	if ok {
		t.Errorf("Accept(\"hello\", 5): expected ok=false, got true")
	}

	if newText != "hello" {
		t.Errorf("Accept(\"hello\", 5): expected newText=%q unchanged, got %q", "hello", newText)
	}

	if newCursor != 5 {
		t.Errorf("Accept(\"hello\", 5): expected newCursor=5 unchanged, got %d", newCursor)
	}
}
