package write_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/internal/tools/write"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// testCtx returns a context carrying a fresh Tracker with default policy.
func testCtx(t *testing.T) (context.Context, *filetime.Tracker) {
	t.Helper()

	tracker := filetime.NewTracker(tool.DefaultReadBeforePolicy())

	return filetime.WithTracker(context.Background(), tracker), tracker
}

func TestWriteNewFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")
	content := "hello world\n"

	tool := write.New()

	got, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": content,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.HasPrefix(got, "W ") {
		t.Errorf("Execute() output = %q, want prefix %q", got, "W ")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(data) != content {
		t.Errorf("file content = %q, want %q", string(data), content)
	}
}

func TestWriteCreatesParentDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "file.txt")

	tool := write.New()

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "nested content\n",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("Stat(%q) error = %v, want file to exist", path, err)
	}
}

func TestWriteExistingWithoutRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")

	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, _ := testCtx(t)
	tool := write.New()

	err := tool.Pre(ctx, map[string]any{
		"path":    path,
		"content": "replacement\n",
	})
	if err == nil {
		t.Fatalf("Pre() error = nil, want error when writing existing file without prior read")
	}

	if !errors.Is(err, filetime.ErrReadRequired) {
		t.Errorf("Pre() error = %v, want wrapping %v", err, filetime.ErrReadRequired)
	}
}

func TestWriteExistingWithRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "readfirst.txt")

	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, tracker := testCtx(t)
	tracker.RecordRead(path)

	tool := write.New()

	got, err := tool.Execute(ctx, map[string]any{
		"path":    path,
		"content": "replacement\n",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.HasPrefix(got, "W ") {
		t.Errorf("Execute() output = %q, want prefix %q", got, "W ")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(data) != "replacement\n" {
		t.Errorf("file content = %q, want %q", string(data), "replacement\n")
	}
}
