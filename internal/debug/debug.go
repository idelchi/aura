package debug

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Logger writes timestamped debug output to <configDir>/debug.log.
// All methods are safe to call on a nil receiver (no-op).
// Methods are protected by a mutex for concurrent use from multiple goroutines.
type Logger struct {
	mu   sync.Mutex
	file *os.File
}

// New returns a Logger if enabled is true, otherwise nil.
// The log file is written to configDir/debug.log.
func New(configDir string, enabled bool) *Logger {
	if !enabled {
		return nil
	}

	folder.New(configDir).Create()

	f, err := file.New(configDir, "debug.log").OpenForWriting()
	if err != nil {
		return nil
	}

	l := &Logger{file: f}
	l.Log("debug logging started (dir=%s)", configDir)

	return l
}

// Log writes a formatted debug line with timestamp.
func (l *Logger) Log(format string, args ...any) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.file, "[DEBUG] %s %s\n", ts, msg)
}

// Span logs the start of a named phase and returns a function that logs its duration.
func (l *Logger) Span(name string) func() {
	if l == nil {
		return func() {}
	}

	l.Log("%s...", name)

	start := time.Now()

	return func() {
		l.Log("%s done (%s)", name, time.Since(start).Round(time.Millisecond))
	}
}

// Close closes the log file.
func (l *Logger) Close() {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.file.Close()
}

// Package-level singleton.
var (
	global *Logger
	once   sync.Once
)

// Init sets up the global logger. Safe to call multiple times — first call wins.
func Init(configDir string, enabled bool) {
	once.Do(func() {
		global = New(configDir, enabled)
	})
}

// Global returns the global logger (may be nil if not initialized or disabled).
func Global() *Logger {
	return global
}

// Log writes to the global logger. No-op if not initialized or disabled.
func Log(format string, args ...any) {
	global.Log(format, args...)
}

// Span starts a timed span on the global logger.
func Span(name string) func() {
	return global.Span(name)
}

// Close closes the global logger.
func Close() {
	global.Close()
}
