package mcp

import (
	"bufio"
	"io"
	"strings"
	"sync"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/sandbox"
)

// Confinement is the Landlock confinement applied to stdio servers configured with
// `sandbox: true`: the session's effective sandbox restrictions, with relative paths
// resolved against WorkDir (the server's working directory).
type Confinement struct {
	WorkDir   string
	ReadOnly  []string
	ReadWrite []string
}

// Spec returns the launcher specification of the confinement.
func (c Confinement) Spec() sandbox.ExecSpec {
	return sandbox.ExecSpec{ReadOnly: c.ReadOnly, ReadWrite: c.ReadWrite}
}

// stderrLines bounds the server stderr kept for diagnostics.
const stderrLines = 20

// Stderr drains a server's stderr so a chatty server can never block on a full pipe,
// logging each line and keeping the most recent ones for error messages.
type Stderr struct {
	name  string
	mu    sync.Mutex
	lines []string
}

// Drain reads r in the background until it closes.
func (s *Stderr) Drain(r io.Reader) {
	go func() {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			debug.Log("[mcp] %s stderr: %s", s.name, line)

			s.mu.Lock()
			s.lines = append(s.lines, line)
			if len(s.lines) > stderrLines {
				s.lines = s.lines[len(s.lines)-stderrLines:]
			}
			s.mu.Unlock()
		}
	}()
}

// Tail returns the most recent stderr lines.
func (s *Stderr) Tail() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return strings.Join(s.lines, "\n")
}
