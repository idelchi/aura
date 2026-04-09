package debug_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/debug"
)

func TestNewDisabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	l := debug.New(dir, false)

	if l != nil {
		t.Errorf("New(dir, false) = %v, want nil", l)
	}
}

func TestNewEnabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	l := debug.New(dir, true)

	if l == nil {
		t.Fatal("New(dir, true) = nil, want non-nil")
	}

	defer l.Close()

	path := filepath.Join(dir, "debug.log")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("debug.log not created: %v", err)
	}
}

func TestLogWritesOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	l := debug.New(dir, true)
	if l == nil {
		t.Fatal("New returned nil")
	}

	l.Log("hello %s", "world")
	l.Close()

	data, err := os.ReadFile(filepath.Join(dir, "debug.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(string(data), "hello world") {
		t.Errorf("log file missing 'hello world', got: %s", string(data))
	}
}

func TestLogTimestamp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	l := debug.New(dir, true)
	if l == nil {
		t.Fatal("New returned nil")
	}

	l.Log("check timestamp")
	l.Close()

	data, err := os.ReadFile(filepath.Join(dir, "debug.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Match [DEBUG] HH:MM:SS.mmm pattern
	re := regexp.MustCompile(`\[DEBUG\] \d{2}:\d{2}:\d{2}\.\d{3} `)
	if !re.Match(data) {
		t.Errorf("log output missing timestamp pattern, got: %s", string(data))
	}
}

func TestNilReceiverLog(t *testing.T) {
	t.Parallel()

	var l *debug.Logger
	// Must not panic
	l.Log("should not panic %s", "test")
}

func TestNilReceiverClose(t *testing.T) {
	t.Parallel()

	var l *debug.Logger
	// Must not panic
	l.Close()
}

func TestCloseFlushes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	l := debug.New(dir, true)
	if l == nil {
		t.Fatal("New returned nil")
	}

	l.Log("flush test")
	l.Close()

	data, err := os.ReadFile(filepath.Join(dir, "debug.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if len(data) == 0 {
		t.Error("log file empty after Close, want content")
	}

	if !strings.Contains(string(data), "flush test") {
		t.Errorf("log file missing 'flush test', got: %s", string(data))
	}
}
