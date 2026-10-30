package mirror

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Stdout writes reach the terminal and the log before Write returns, unlike writes through
// os.Stdout, which the tee goroutine forwards later. Callers rely on this to keep text in
// order with their own direct terminal writes.
func TestStdoutWritesTerminalAndLogSynchronously(t *testing.T) {
	terminal, err := os.CreateTemp(t.TempDir(), "terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()

	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = terminal, terminal

	defer func() { os.Stdout, os.Stderr = stdout, stderr }()

	path := filepath.Join(t.TempDir(), "output.log")

	m, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if _, err := fmt.Fprint(m.Stdout(), "\x1b[36m[Tool: Read]\x1b[0m\n"); err != nil {
		t.Fatal(err)
	}

	// No Close, no waiting: both copies must already be complete.
	logged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(logged) != "[Tool: Read]\n" {
		t.Errorf("log = %q, want the stripped text immediately", logged)
	}

	if _, err := terminal.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	screen, err := io.ReadAll(terminal)
	if err != nil {
		t.Fatal(err)
	}

	if string(screen) != "\x1b[36m[Tool: Read]\x1b[0m\n" {
		t.Errorf("terminal = %q, want the styled text immediately", screen)
	}
}
