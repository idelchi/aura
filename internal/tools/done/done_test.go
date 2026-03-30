package done_test

import (
	"context"
	"testing"

	"github.com/idelchi/aura/internal/tools/done"
)

func TestDoneWithCallback(t *testing.T) {
	t.Parallel()

	var received string

	tool := done.New(func(summary string) {
		received = summary
	})

	result, err := tool.Execute(context.Background(), map[string]any{
		"summary": "task complete",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if received != "task complete" {
		t.Errorf("callback received %q, want %q", received, "task complete")
	}

	if result != "Done" {
		t.Errorf("result = %q, want %q", result, "Done")
	}
}

func TestDoneNilCallback(t *testing.T) {
	t.Parallel()

	tool := done.New(nil)

	// Should not panic when OnDone is nil.
	result, err := tool.Execute(context.Background(), map[string]any{
		"summary": "finished",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result != "Done" {
		t.Errorf("result = %q, want %q", result, "Done")
	}
}

// TestDoneEmptySummary verifies that an empty args map is accepted — the
// Inputs.Summary field carries only a jsonschema annotation, not a
// validate:"required" tag, so the validator does not reject an empty string.
// The callback receives "" and Execute still returns "Done".
func TestDoneEmptySummary(t *testing.T) {
	t.Parallel()

	var received string

	tool := done.New(func(summary string) {
		received = summary
	})

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if received != "" {
		t.Errorf("callback received %q, want empty string", received)
	}

	if result != "Done" {
		t.Errorf("result = %q, want %q", result, "Done")
	}
}
