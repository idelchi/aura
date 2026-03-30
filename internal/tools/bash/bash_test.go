package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/tools/bash"
)

//go:fix inline
func intPtr(n int) *int { return new(n) }

func TestExecuteEcho(t *testing.T) {
	t.Parallel()

	tool := bash.New(config.BashTruncation{}, "")

	out, err := tool.Execute(context.Background(), map[string]any{"command": "echo hello"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want it to contain %q", out, "hello")
	}
}

func TestExecuteExitCode(t *testing.T) {
	t.Parallel()

	tool := bash.New(config.BashTruncation{}, "")

	out, err := tool.Execute(context.Background(), map[string]any{"command": "exit 1"})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (non-zero exit is not an error)", err)
	}

	if !strings.Contains(out, "EXIT CODE: 1") {
		t.Errorf("output = %q, want it to contain %q", out, "EXIT CODE: 1")
	}
}

func TestExecuteSyntaxError(t *testing.T) {
	t.Parallel()

	tool := bash.New(config.BashTruncation{}, "")

	_, err := tool.Execute(context.Background(), map[string]any{"command": "if then"})
	if err == nil {
		t.Fatal("Execute() error = nil, want non-nil for syntax error")
	}

	if !strings.Contains(err.Error(), "syntax error") {
		t.Errorf("err = %q, want it to contain %q", err.Error(), "syntax error")
	}
}

func TestExecuteWorkdir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tool := bash.New(config.BashTruncation{}, "")

	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "pwd",
		"workdir": dir,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, dir) {
		t.Errorf("output = %q, want it to contain %q", out, dir)
	}
}

func TestExecuteStderr(t *testing.T) {
	t.Parallel()

	tool := bash.New(config.BashTruncation{}, "")

	out, err := tool.Execute(context.Background(), map[string]any{"command": "echo err >&2"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "STDERR:") {
		t.Errorf("output = %q, want it to contain %q", out, "STDERR:")
	}
}

func TestExecuteTimeout(t *testing.T) {
	t.Parallel()

	tool := bash.New(config.BashTruncation{}, "")

	out, err := tool.Execute(context.Background(), map[string]any{
		"command":    "sleep 60",
		"timeout_ms": int64(100),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (timeout is not an error return)", err)
	}

	if !strings.Contains(out, "TIMEOUT") {
		t.Errorf("output = %q, want it to contain %q", out, "TIMEOUT")
	}
}

func TestTruncateOutputDisabled(t *testing.T) {
	t.Parallel()

	// nil MaxLines means truncation is disabled — full output must be returned.
	tool := bash.New(config.BashTruncation{}, "")

	out, err := tool.Execute(context.Background(), map[string]any{"command": "seq 1 50"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// All 50 numbers must be present.
	for _, want := range []string{"1", "25", "50"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q (truncation should be disabled)", want)
		}
	}

	if strings.Contains(out, "truncated") {
		t.Errorf("output contains %q but truncation should be disabled", "truncated")
	}
}

func TestTruncateOutputBelowThreshold(t *testing.T) {
	t.Parallel()

	// 5 lines generated, MaxLines=10 — below threshold, full output expected.
	cfg := config.BashTruncation{
		MaxLines: new(10),
	}
	tool := bash.New(cfg, "")

	out, err := tool.Execute(context.Background(), map[string]any{"command": "seq 1 5"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, want := range []string{"1", "2", "3", "4", "5"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q (below threshold, should be full output)", want)
		}
	}

	if strings.Contains(out, "truncated") {
		t.Errorf("output contains %q but output is below threshold", "truncated")
	}
}

func TestTruncateOutputAboveThreshold(t *testing.T) {
	t.Parallel()

	// 20 lines generated, MaxLines=10, HeadLines=3, TailLines=3.
	// Expect head (1,2,3), tail (18,19,20), truncation notice, but NOT all middle lines.
	cfg := config.BashTruncation{
		MaxLines:  new(10),
		HeadLines: new(3),
		TailLines: new(3),
	}
	tool := bash.New(cfg, "")

	out, err := tool.Execute(context.Background(), map[string]any{"command": "seq 1 20"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out, "truncated") {
		t.Errorf("output = %q, want it to contain %q", out, "truncated")
	}

	// Head lines must be present (seq output: "1\n2\n3\n...").
	for _, want := range []string{"1\n", "2\n", "3\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing head line %q", want)
		}
	}

	// seq 1 20 ends with a trailing newline, so strings.Split yields 21 elements
	// (last is ""). With TailLines=3 the tail window is ["19", "20", ""], meaning
	// "19" and "20" are the last real numbers kept.
	for _, want := range []string{"19\n", "20\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing tail line %q", want)
		}
	}

	// Middle lines must NOT appear as isolated line values.
	// "10" and "11" sit well inside the truncated region and are not substrings
	// of any head/tail number (1–3, 19–20), so their absence is unambiguous.
	for _, absent := range []string{"\n10\n", "\n11\n"} {
		if strings.Contains(out, absent) {
			t.Errorf("output contains middle line %q but it should be truncated", absent)
		}
	}
}
