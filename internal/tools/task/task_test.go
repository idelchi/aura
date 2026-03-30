package task_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/subagent"
	"github.com/idelchi/aura/internal/tools/task"
)

func TestFormatResultBasic(t *testing.T) {
	t.Parallel()

	got := task.FormatResult("researcher", subagent.Result{
		Text:      "done",
		ToolCalls: 3,
	})

	if !strings.Contains(got, `[Subagent "researcher" completed in 3 tool calls`) {
		t.Errorf("missing header, got: %s", got)
	}

	if !strings.Contains(got, "done") {
		t.Errorf("missing text body, got: %s", got)
	}
}

func TestFormatResultEmptyAgent(t *testing.T) {
	t.Parallel()

	got := task.FormatResult("", subagent.Result{
		Text:      "result",
		ToolCalls: 1,
	})

	if !strings.Contains(got, `"default"`) {
		t.Errorf("expected 'default' label for empty agent, got: %s", got)
	}
}

func TestFormatResultNoTools(t *testing.T) {
	t.Parallel()

	got := task.FormatResult("agent", subagent.Result{
		Text:      "ok",
		ToolCalls: 2,
	})

	// Extract header line (first line)
	header := strings.SplitN(got, "\n", 2)[0]
	if strings.Contains(header, "(") {
		t.Errorf("no tools should have no parenthetical, got header: %s", header)
	}
}

func TestFormatResultWithTools(t *testing.T) {
	t.Parallel()

	got := task.FormatResult("agent", subagent.Result{
		Text:      "ok",
		ToolCalls: 7,
		Tools:     map[string]int{"Read": 5, "Grep": 2},
	})

	if !strings.Contains(got, "Read x5") {
		t.Errorf("missing 'Read x5', got: %s", got)
	}

	if !strings.Contains(got, "Grep x2") {
		t.Errorf("missing 'Grep x2', got: %s", got)
	}

	// Read (5) should come before Grep (2) — sorted by count desc
	readIdx := strings.Index(got, "Read x5")

	grepIdx := strings.Index(got, "Grep x2")
	if readIdx > grepIdx {
		t.Errorf("Read x5 should appear before Grep x2 (count desc), got: %s", got)
	}
}

func TestFormatResultToolTies(t *testing.T) {
	t.Parallel()

	got := task.FormatResult("agent", subagent.Result{
		Text:      "ok",
		ToolCalls: 6,
		Tools:     map[string]int{"Bb": 3, "Aa": 3},
	})

	// Same count → alphabetical: Aa before Bb
	aaIdx := strings.Index(got, "Aa x3")

	bbIdx := strings.Index(got, "Bb x3")
	if aaIdx < 0 || bbIdx < 0 {
		t.Fatalf("missing tool entries, got: %s", got)
	}

	if aaIdx > bbIdx {
		t.Errorf("Aa x3 should appear before Bb x3 (alphabetical on tie), got: %s", got)
	}
}

func TestFormatResultSingleTool(t *testing.T) {
	t.Parallel()

	got := task.FormatResult("agent", subagent.Result{
		Text:      "ok",
		ToolCalls: 1,
		Tools:     map[string]int{"Bash": 1},
	})

	if !strings.Contains(got, "(Bash x1)") {
		t.Errorf("missing '(Bash x1)', got: %s", got)
	}
}
