// Package mirror duplicates os.Stdout and os.Stderr to a file.
// ANSI escape sequences are stripped from the file copy, producing clean text.
package mirror

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/charmbracelet/x/ansi"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/godyl/pkg/path/file"
)

// Mirror captures os.Stdout and os.Stderr writes and duplicates them to a file.
// The terminal gets raw bytes (with ANSI colors); the file gets stripped plain text.
// Create with New, close with Close to restore original descriptors.
type Mirror struct {
	file        *os.File
	origStdout  *os.File
	origStderr  *os.File
	stdoutWrite *os.File
	stderrWrite *os.File
	wg          sync.WaitGroup
	mu          sync.Mutex
}

// New creates a mirror that duplicates stdout and stderr to the file at path.
// The file is truncated on creation. ANSI escape sequences are stripped from
// the file copy so it contains clean, readable text.
// Call Close to restore original stdout/stderr and flush remaining output.
func New(path string) (*Mirror, error) {
	f, err := file.New(path).OpenForWriting(0o644)
	if err != nil {
		return nil, fmt.Errorf("opening output file: %w", err)
	}

	m := &Mirror{
		file:       f,
		origStdout: os.Stdout,
		origStderr: os.Stderr,
	}

	// Redirect stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		f.Close()

		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	m.stdoutWrite = stdoutW
	os.Stdout = stdoutW

	m.wg.Add(1)

	go m.tee(stdoutR, m.origStdout)

	// Redirect stderr
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		os.Stdout = m.origStdout

		stdoutW.Close()
		f.Close()

		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	m.stderrWrite = stderrW
	os.Stderr = stderrW

	m.wg.Add(1)

	go m.tee(stderrR, m.origStderr)

	return m, nil
}

// OrigStdout returns the original os.Stdout that was captured before the mirror replaced it.
func (m *Mirror) OrigStdout() *os.File { return m.origStdout }

// OrigStderr preserves terminal progress output when stderr is mirrored to a pipe.
func (m *Mirror) OrigStderr() *os.File { return m.origStderr }

// Stdout returns a writer that writes synchronously to the original stdout and the file.
// Writes through os.Stdout reach the terminal later, via the tee goroutine; use this writer
// for output whose order relative to direct terminal writes (e.g. readline redraws) matters.
func (m *Mirror) Stdout() io.Writer { return io.MultiWriter(m.origStdout, m) }

// Write records semantic transcript text without echoing it to the terminal.
// It shares the file with mirrored stdout/stderr and serializes complete writes.
func (m *Mirror) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := io.WriteString(m.file, ansi.Strip(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close restores original stdout/stderr, flushes remaining pipe data, and closes the file.
func (m *Mirror) Close() {
	// Restore originals first — new writes bypass the pipe.
	os.Stdout = m.origStdout
	os.Stderr = m.origStderr

	// Close pipe write ends — triggers EOF on tee goroutines.
	m.stdoutWrite.Close()
	m.stderrWrite.Close()

	// Wait for goroutines to finish flushing.
	m.wg.Wait()

	m.file.Close()
}

// tee reads from r and writes raw bytes to orig (terminal) and
// ANSI-stripped bytes to the mirror file.
func (m *Mirror) tee(r, orig *os.File) {
	defer m.wg.Done()

	w := io.MultiWriter(orig, m)

	if _, err := io.Copy(w, r); err != nil {
		debug.Log("[mirror] tee copy: %v", err)
	}

	if err := r.Close(); err != nil {
		debug.Log("[mirror] tee close: %v", err)
	}
}
