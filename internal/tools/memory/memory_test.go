package memory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/tools/memory/read"
	"github.com/idelchi/aura/internal/tools/memory/write"
)

func TestMemoryWriteAndRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTool := write.New(dir, dir)
	readTool := read.New(dir, dir)

	const (
		key     = "test-key"
		content = "# Test\nSome content"
	)

	_, err := writeTool.Execute(context.Background(), map[string]any{
		"key":     key,
		"content": content,
	})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	got, err := readTool.Execute(context.Background(), map[string]any{
		"key": key,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if got != content {
		t.Errorf("content mismatch\ngot:  %q\nwant: %q", got, content)
	}
}

func TestMemoryList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTool := write.New(dir, dir)
	readTool := read.New(dir, dir)

	entries := []struct {
		key     string
		content string
	}{
		{"alpha", "# Alpha\nFirst entry"},
		{"beta", "# Beta\nSecond entry"},
	}

	for _, e := range entries {
		_, err := writeTool.Execute(context.Background(), map[string]any{
			"key":     e.key,
			"content": e.content,
		})
		if err != nil {
			t.Fatalf("write %q failed: %v", e.key, err)
		}
	}

	got, err := readTool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	for _, e := range entries {
		if !strings.Contains(got, e.key) {
			t.Errorf("list output missing key %q\ngot: %s", e.key, got)
		}
	}
}

func TestMemorySearch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTool := write.New(dir, dir)
	readTool := read.New(dir, dir)

	_, err := writeTool.Execute(context.Background(), map[string]any{
		"key":     "db-notes",
		"content": "# DB Notes\nWe use a PostgreSQL database for storage.",
	})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	got, err := readTool.Execute(context.Background(), map[string]any{
		"query": "database",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if !strings.Contains(got, "db-notes") {
		t.Errorf("search result missing expected key\ngot: %s", got)
	}

	if strings.HasPrefix(got, "No matches") {
		t.Errorf("search reported no matches but expected to find \"database\"\ngot: %s", got)
	}
}

func TestMemoryReadMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	readTool := read.New(dir, dir)

	_, err := readTool.Execute(context.Background(), map[string]any{
		"key": "does-not-exist",
	})
	if err == nil {
		t.Error("expected error reading non-existent key, got nil")
	}
}

func TestMemoryInvalidKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTool := write.New(dir, dir)

	_, err := writeTool.Execute(context.Background(), map[string]any{
		"key":     "invalid/key",
		"content": "test",
	})
	if err == nil {
		t.Error("expected error for key containing path separator, got nil")
	}
}
