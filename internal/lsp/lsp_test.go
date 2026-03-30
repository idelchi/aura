package lsp

import (
	"testing"

	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

func TestTimeoutOrDefault(t *testing.T) {
	t.Parallel()

	if got := (Server{Timeout: 0}).TimeoutOrDefault(); got != 30 {
		t.Errorf("Server{Timeout: 0}.TimeoutOrDefault() = %d, want 30", got)
	}

	if got := (Server{Timeout: 60}).TimeoutOrDefault(); got != 60 {
		t.Errorf("Server{Timeout: 60}.TimeoutOrDefault() = %d, want 60", got)
	}
}

func TestFileExtension(t *testing.T) {
	t.Parallel()

	if got := fileExtension("main.go"); got != "go" {
		t.Errorf("fileExtension(%q) = %q, want %q", "main.go", got, "go")
	}

	if got := fileExtension("Makefile"); got != "" {
		t.Errorf("fileExtension(%q) = %q, want %q", "Makefile", got, "")
	}
}

func TestServerStateString(t *testing.T) {
	t.Parallel()

	if got := StateReady.String(); got != "ready" {
		t.Errorf("StateReady.String() = %q, want %q", got, "ready")
	}

	if got := StateError.String(); got != "error" {
		t.Errorf("StateError.String() = %q, want %q", got, "error")
	}
}

func TestDiagnosticSeverity(t *testing.T) {
	t.Parallel()

	if got := diagnosticSeverity(protocol.SeverityError); got != "error" {
		t.Errorf("diagnosticSeverity(SeverityError) = %q, want %q", got, "error")
	}

	if got := diagnosticSeverity(protocol.SeverityWarning); got != "warning" {
		t.Errorf("diagnosticSeverity(SeverityWarning) = %q, want %q", got, "warning")
	}
}

func TestResolvePath(t *testing.T) {
	t.Parallel()

	m := &Manager{workDir: "/tmp/project"}

	if got := m.ResolvePath("src/main.go"); got != "/tmp/project/src/main.go" {
		t.Errorf("ResolvePath(%q) = %q, want %q", "src/main.go", got, "/tmp/project/src/main.go")
	}

	if got := m.ResolvePath("/absolute/path"); got != "/absolute/path" {
		t.Errorf("ResolvePath(%q) = %q, want %q", "/absolute/path", got, "/absolute/path")
	}
}
