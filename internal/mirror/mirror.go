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

	stripped := &stripWriter{w: m.file}
	w := io.MultiWriter(orig, stripped)

	if _, err := io.Copy(w, r); err != nil {
		debug.Log("[mirror] tee copy: %v", err)
	}

	if err := r.Close(); err != nil {
		debug.Log("[mirror] tee close: %v", err)
	}
}

// stripWriter wraps an io.Writer and strips ANSI escape sequences before writing.
type stripWriter struct {
	w io.Writer
}

func (s *stripWriter) Write(p []byte) (int, error) {
	clean := ansi.Strip(string(p))

	_, err := s.w.Write([]byte(clean))
	if err != nil {
		return 0, err
	}

	// Return original length — callers expect len(p) on success.
	return len(p), nil
}
