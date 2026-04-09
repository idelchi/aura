package mkdir_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/tools/mkdir"
)

func TestMkdirSingle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "newdir")

	tool := mkdir.New()

	_, err := tool.Execute(context.Background(), map[string]any{
		"paths": []any{target},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v, want directory to exist", target, err)
	}

	if !info.IsDir() {
		t.Errorf("Stat(%q).IsDir() = false, want true", target)
	}
}

func TestMkdirNested(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c")

	tool := mkdir.New()

	_, err := tool.Execute(context.Background(), map[string]any{
		"paths": []any{target},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v, want nested directory to exist", target, err)
	}

	if !info.IsDir() {
		t.Errorf("Stat(%q).IsDir() = false, want true", target)
	}
}

func TestMkdirAlreadyExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "existing")

	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", target, err)
	}

	tool := mkdir.New()

	got, err := tool.Execute(context.Background(), map[string]any{
		"paths": []any{target},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil when directory already exists", err)
	}

	if !strings.Contains(strings.ToLower(got), "already exist") {
		t.Errorf("Execute() = %q, want output to contain %q", got, "already exist")
	}
}

func TestMkdirPathIsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "regularfile.txt")

	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	tool := mkdir.New()

	_, err := tool.Execute(context.Background(), map[string]any{
		"paths": []any{path},
	})
	if err == nil {
		t.Fatalf("Execute() error = nil, want error when path is an existing file")
	}
}
