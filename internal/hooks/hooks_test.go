package hooks

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestRunPreMatchAll(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{Name: "all", Matcher: nil, Command: `echo ok`, Timeout: 5 * time.Second},
		},
	}

	result := r.RunPre(context.Background(), "Anything", nil, t.TempDir())

	if result.Blocked {
		t.Errorf("expected Blocked=false, got true")
	}
}

func TestRunPreMatchRegex(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{
				Name:    "bash-only",
				Matcher: regexp.MustCompile(`^Bash$`),
				Command: `echo matched`,
				Timeout: 5 * time.Second,
			},
		},
	}

	cwd := t.TempDir()

	// Should match Bash.
	result := r.RunPre(context.Background(), "Bash", nil, cwd)
	if !strings.Contains(result.Message, "matched") {
		t.Errorf("expected Message to contain %q, got %q", "matched", result.Message)
	}

	// Should not match Read — expect empty result.
	result = r.RunPre(context.Background(), "Read", nil, cwd)
	if result.Blocked {
		t.Errorf("expected Blocked=false for non-matching tool, got true")
	}

	if result.Message != "" {
		t.Errorf("expected empty Message for non-matching tool, got %q", result.Message)
	}
}

func TestRunPreNoMatch(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{
				Name:    "write-only",
				Matcher: regexp.MustCompile(`^Write$`),
				Command: `echo write`,
				Timeout: 5 * time.Second,
			},
		},
	}

	result := r.RunPre(context.Background(), "Read", nil, t.TempDir())

	if result.Blocked {
		t.Errorf("expected Blocked=false, got true")
	}

	if result.Message != "" {
		t.Errorf("expected empty Message, got %q", result.Message)
	}
}

func TestRunPreExitZero(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{Name: "exit-zero", Matcher: nil, Command: `echo ok`, Timeout: 5 * time.Second},
		},
	}

	result := r.RunPre(context.Background(), "Bash", nil, t.TempDir())

	if result.Blocked {
		t.Errorf("expected Blocked=false, got true")
	}

	if !strings.Contains(result.Message, "ok") {
		t.Errorf("expected Message to contain %q, got %q", "ok", result.Message)
	}
}

func TestRunPreExitTwo(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{Name: "blocker", Matcher: nil, Command: `echo blocked >&2; exit 2`, Timeout: 5 * time.Second},
		},
	}

	result := r.RunPre(context.Background(), "Bash", nil, t.TempDir())

	if !result.Blocked {
		t.Errorf("expected Blocked=true, got false")
	}

	if !strings.Contains(result.Message, "blocked") {
		t.Errorf("expected Message to contain %q, got %q", "blocked", result.Message)
	}
}

func TestRunPreExitOne(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{Name: "feedback", Matcher: nil, Command: `echo feedback >&2; exit 1`, Timeout: 5 * time.Second},
		},
	}

	result := r.RunPre(context.Background(), "Bash", nil, t.TempDir())

	if result.Blocked {
		t.Errorf("expected Blocked=false, got true")
	}

	if !strings.Contains(result.Message, "feedback") {
		t.Errorf("expected Message to contain %q, got %q", "feedback", result.Message)
	}
}

func TestRunPreJSONDeny(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{
				Name:    "json-deny",
				Matcher: nil,
				Command: `echo '{"deny":true,"reason":"custom reason"}'`,
				Timeout: 5 * time.Second,
			},
		},
	}

	result := r.RunPre(context.Background(), "Bash", nil, t.TempDir())

	if !result.Blocked {
		t.Errorf("expected Blocked=true, got false")
	}

	if !strings.Contains(result.Message, "custom reason") {
		t.Errorf("expected Message to contain %q, got %q", "custom reason", result.Message)
	}
}

func TestRunPreTimeout(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{Name: "sleeper", Matcher: nil, Command: `sleep 60`, Timeout: 100 * time.Millisecond},
		},
	}

	start := time.Now()
	result := r.RunPre(context.Background(), "Bash", nil, t.TempDir())
	elapsed := time.Since(start)

	if elapsed >= 2*time.Second {
		t.Errorf("expected RunPre to complete in under 2s, took %v", elapsed)
	}

	if result.Blocked {
		t.Errorf("expected Blocked=false after timeout, got true")
	}

	if !strings.Contains(result.Message, "timed out") {
		t.Errorf("expected timeout message, got %q", result.Message)
	}
}

func TestRunPostBasic(t *testing.T) {
	t.Parallel()

	r := Runner{
		Post: []Entry{
			{Name: "post-hook", Matcher: nil, Command: `echo post-ok`, Timeout: 5 * time.Second},
		},
	}

	result := r.RunPost(context.Background(), "Bash", nil, "some output", t.TempDir())

	if result.Blocked {
		t.Errorf("expected Blocked=false, got true")
	}

	if !strings.Contains(result.Message, "post-ok") {
		t.Errorf("expected Message to contain %q, got %q", "post-ok", result.Message)
	}
}

func TestRunPreFileGlob(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{Name: "go-files", Matcher: nil, Files: "*.go", Command: `echo matched`, Timeout: 5 * time.Second},
		},
	}

	toolInput := map[string]any{"path": "/src/main.go"}
	result := r.RunPre(context.Background(), "Read", toolInput, t.TempDir())

	if !strings.Contains(result.Message, "matched") {
		t.Errorf("expected Message to contain %q, got %q", "matched", result.Message)
	}
}

func TestRunPreFileGlobNoMatch(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{Name: "py-files", Matcher: nil, Files: "*.py", Command: `echo matched`, Timeout: 5 * time.Second},
		},
	}

	toolInput := map[string]any{"path": "/src/main.go"}
	result := r.RunPre(context.Background(), "Read", toolInput, t.TempDir())

	if result.Blocked {
		t.Errorf("expected Blocked=false, got true")
	}

	if result.Message != "" {
		t.Errorf("expected empty Message for glob mismatch, got %q", result.Message)
	}
}

func TestRunPreMultipleEntries(t *testing.T) {
	t.Parallel()

	r := Runner{
		Pre: []Entry{
			{
				Name:    "write-only",
				Matcher: regexp.MustCompile(`^Write$`),
				Command: `echo first`,
				Timeout: 5 * time.Second,
			},
			{Name: "catch-all", Matcher: nil, Command: `echo second`, Timeout: 5 * time.Second},
		},
	}

	result := r.RunPre(context.Background(), "Read", nil, t.TempDir())

	// The first entry should be skipped (matcher requires Write), only second runs.
	if strings.Contains(result.Message, "first") {
		t.Errorf("expected first entry to be skipped, but Message contains %q: %q", "first", result.Message)
	}

	if !strings.Contains(result.Message, "second") {
		t.Errorf("expected Message to contain %q, got %q", "second", result.Message)
	}

	// Also verify the hook name wrapping is present.
	if !strings.Contains(result.Message, "catch-all") {
		t.Errorf("expected Message to contain hook name %q, got %q", "catch-all", result.Message)
	}
}
