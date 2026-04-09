package todo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/todo"
	todotool "github.com/idelchi/aura/internal/tools/todo"
)

// newTools returns a fresh set of todo tools backed by a new empty list.
func newTools(t *testing.T) (create *todotool.Create, list *todotool.List, progress *todotool.Progress) {
	t.Helper()

	l := todo.New()

	return todotool.NewCreate(l), todotool.NewList(l), todotool.NewProgress(l)
}

func TestTodoCreateAndList(t *testing.T) {
	t.Parallel()

	createTool, listTool, _ := newTools(t)

	_, err := createTool.Execute(context.Background(), map[string]any{
		"summary": "Test plan",
		"items": []any{
			map[string]any{"content": "step 1"},
			map[string]any{"content": "step 2"},
		},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	got, err := listTool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if !strings.Contains(got, "step 1") {
		t.Errorf("list missing \"step 1\"\ngot: %s", got)
	}

	if !strings.Contains(got, "step 2") {
		t.Errorf("list missing \"step 2\"\ngot: %s", got)
	}

	// First item should be auto-set to in_progress after create.
	if !strings.Contains(got, "in_progress") {
		t.Errorf("expected at least one item in_progress after create\ngot: %s", got)
	}
}

func TestTodoListEmpty(t *testing.T) {
	t.Parallel()

	_, listTool, _ := newTools(t)

	got, err := listTool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if got != "No todos." {
		t.Errorf("got %q, want %q", got, "No todos.")
	}
}

func TestTodoProgress(t *testing.T) {
	t.Parallel()

	createTool, listTool, progressTool := newTools(t)

	_, err := createTool.Execute(context.Background(), map[string]any{
		"summary": "Progress test",
		"items": []any{
			map[string]any{"content": "item one"},
			map[string]any{"content": "item two"},
		},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Complete item 1; item 2 should auto-advance to in_progress.
	_, err = progressTool.Execute(context.Background(), map[string]any{
		"updates": []any{
			map[string]any{"index": 1, "status": "completed"},
		},
	})
	if err != nil {
		t.Fatalf("progress failed: %v", err)
	}

	got, err := listTool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if !strings.Contains(got, "completed") {
		t.Errorf("expected item 1 to be completed\ngot: %s", got)
	}

	// Item 2 should now be in_progress due to auto-promotion.
	if !strings.Contains(got, "in_progress") {
		t.Errorf("expected item 2 to be auto-promoted to in_progress\ngot: %s", got)
	}
}

func TestTodoProgressInvalidIndex(t *testing.T) {
	t.Parallel()

	createTool, _, progressTool := newTools(t)

	_, err := createTool.Execute(context.Background(), map[string]any{
		"summary": "Small list",
		"items": []any{
			map[string]any{"content": "only item"},
		},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_, err = progressTool.Execute(context.Background(), map[string]any{
		"updates": []any{
			map[string]any{"index": 99, "status": "completed"},
		},
	})
	if err == nil {
		t.Error("expected error for out-of-range index 99, got nil")
	}
}

func TestTodoCreateSummaryOnly(t *testing.T) {
	t.Parallel()

	createTool, _, _ := newTools(t)

	_, err := createTool.Execute(context.Background(), map[string]any{
		"summary": "Just a summary",
	})
	if err != nil {
		t.Errorf("unexpected error creating with summary only: %v", err)
	}
}

func TestTodoCreateEmpty(t *testing.T) {
	t.Parallel()

	createTool, _, _ := newTools(t)

	_, err := createTool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error when creating with neither summary nor items, got nil")
	}
}
