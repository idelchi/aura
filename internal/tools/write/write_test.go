package write_test

import (
	"context"
	"errors"
	"fmt"
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

func TestWriteEmptyContent(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "empty.txt")
			if existing {
				if err := os.WriteFile(path, []byte("remove me\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, tracker := testCtx(t)
			args := map[string]any{"path": path, "content": ""}
			w := write.New()
			if existing {
				if err := w.Pre(ctx, args); !errors.Is(err, filetime.ErrReadRequired) {
					t.Fatalf("empty overwrite bypassed read-before-write: %v", err)
				}
				tracker.RecordRead(path)
			}
			if err := w.Pre(ctx, args); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Preview(ctx, args); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Execute(ctx, args); err != nil {
				t.Fatal(err)
			}
			w.Post(ctx, args)
			data, err := os.ReadFile(path)
			if err != nil || len(data) != 0 || !tracker.WasRead(path) {
				t.Fatalf("empty write: bytes=%q read=%v error=%v", data, tracker.WasRead(path), err)
			}
		})
	}
}

func TestWriteRequiresContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(path, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"path": path}, {"path": path, "content": nil}, {"path": path, "content": 42},
	} {
		w := write.New()
		if _, err := w.Preview(t.Context(), args); err == nil {
			t.Fatal("preview accepted missing or invalid content")
		}
		if _, err := w.Execute(t.Context(), args); err == nil {
			t.Fatal("execution accepted missing or invalid content")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "keep\n" {
			t.Fatal("invalid write changed existing content")
		}
	}
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
