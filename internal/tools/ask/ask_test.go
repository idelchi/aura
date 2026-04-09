package ask_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/tools/ask"
)

func TestExecuteBasic(t *testing.T) {
	t.Parallel()

	var got ask.Request

	tool := ask.New(func(_ context.Context, req ask.Request) (string, error) {
		got = req

		return "noted", nil
	})

	result, err := tool.Execute(context.Background(), map[string]any{
		"question": "What color?",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result != "noted" {
		t.Errorf("result = %q, want %q", result, "noted")
	}

	if got.Question != "What color?" {
		t.Errorf("Question = %q, want %q", got.Question, "What color?")
	}

	if got.Options != nil {
		t.Errorf("Options = %v, want nil", got.Options)
	}
}

func TestExecuteWithOptions(t *testing.T) {
	t.Parallel()

	var got ask.Request

	tool := ask.New(func(_ context.Context, req ask.Request) (string, error) {
		got = req

		return "ok", nil
	})

	_, err := tool.Execute(context.Background(), map[string]any{
		"question": "Pick one",
		"options": []any{
			map[string]any{"label": "A", "description": "Option A"},
			map[string]any{"label": "B", "description": "Option B"},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(got.Options) != 2 {
		t.Fatalf("len(Options) = %d, want 2", len(got.Options))
	}

	if got.Options[0].Label != "A" {
		t.Errorf("Options[0].Label = %q, want %q", got.Options[0].Label, "A")
	}

	if got.Options[1].Label != "B" {
		t.Errorf("Options[1].Label = %q, want %q", got.Options[1].Label, "B")
	}
}

func TestExecuteMultiSelect(t *testing.T) {
	t.Parallel()

	var got ask.Request

	tool := ask.New(func(_ context.Context, req ask.Request) (string, error) {
		got = req

		return "ok", nil
	})

	_, err := tool.Execute(context.Background(), map[string]any{
		"question":     "Pick many",
		"multi_select": true,
		"options": []any{
			map[string]any{"label": "X"},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !got.MultiSelect {
		t.Error("MultiSelect = false, want true")
	}
}

func TestExecuteMissingQuestion(t *testing.T) {
	t.Parallel()

	tool := ask.New(func(_ context.Context, req ask.Request) (string, error) {
		return "should not reach", nil
	})

	_, err := tool.Execute(context.Background(), map[string]any{
		"question": "",
	})
	if err == nil {
		t.Fatal("expected error for empty question, got nil")
	}

	if !strings.Contains(err.Error(), "question") {
		t.Errorf("error = %q, want it to mention 'question'", err.Error())
	}
}

func TestExecuteCallbackError(t *testing.T) {
	t.Parallel()

	tool := ask.New(func(_ context.Context, req ask.Request) (string, error) {
		return "", errors.New("user cancelled")
	})

	_, err := tool.Execute(context.Background(), map[string]any{
		"question": "Continue?",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "user cancelled") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "user cancelled")
	}
}

func TestExecuteCallbackResult(t *testing.T) {
	t.Parallel()

	tool := ask.New(func(_ context.Context, req ask.Request) (string, error) {
		return "user chose PostgreSQL", nil
	})

	result, err := tool.Execute(context.Background(), map[string]any{
		"question": "Which DB?",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result != "user chose PostgreSQL" {
		t.Errorf("result = %q, want %q", result, "user chose PostgreSQL")
	}
}
