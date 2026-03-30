package lsp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/powernap/pkg/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// diagnosticWaitTimeout is the maximum time to wait for diagnostics after a change.
// gopls delivers diagnostics in ~1-5ms once warmed up. 2s is generous for the hot path;
// cold starts (30-60s workspace load) will miss this window regardless.
const diagnosticWaitTimeout = 2 * time.Second

// Manager manages LSP server lifecycle with lazy initialization.
// Servers are started on first file touch matching their file types and root markers.
type Manager struct {
	mu      sync.Mutex
	clients map[string]*Client // server name → running client
	config  Config
	workDir string
}

// NewManager creates a manager with the given configuration.
// No servers are started until Start() or NotifyChange() is called.
func NewManager(cfg Config, workDir string) *Manager {
	return &Manager{
		clients: make(map[string]*Client),
		config:  cfg,
		workDir: workDir,
	}
}

// Start lazily starts LSP servers that handle the given file path.
// Already-running servers are skipped. Safe for concurrent use.
func (m *Manager) Start(ctx context.Context, filePath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	f := file.New(filePath)

	debug.Log("[lsp] Start called: file=%s workDir=%s servers=%d", filePath, m.workDir, len(m.config.Servers))

	for name, server := range m.config.Servers {
		if server.Disabled {
			debug.Log("[lsp] %s: skipped (disabled)", name)

			continue
		}

		if _, running := m.clients[name]; running {
			continue
		}

		if !m.HandlesFile(server, filePath) {
			debug.Log("[lsp] %s: skipped (file %q not handled, types=%v)", name, f.Base(), server.FileTypes)

			continue
		}

		if !m.HasRootMarkers(server) {
			debug.Log("[lsp] %s: skipped (root markers %v not found in %s)", name, server.RootMarkers, m.workDir)

			continue
		}

		debug.Log("[lsp] starting %s for %s", name, f.Base())

		rootURI := "file://" + m.workDir

		client, err := NewClient(ctx, name, server, rootURI)
		if err != nil {
			debug.Log("[lsp] failed to start %s: %v", name, err)

			continue
		}

		m.clients[name] = client
		debug.Log("[lsp] %s ready", name)
	}
}

// NotifyChange opens the file on demand, sends a change notification,
// and waits for diagnostics on all clients handling the file.
// This is the primary entry point for Patch/Write tools.
func (m *Manager) NotifyChange(ctx context.Context, path string) {
	path = m.ResolvePath(path)
	m.Start(ctx, path)

	m.mu.Lock()
	clients := m.clientsForFile(path)
	m.mu.Unlock()

	for _, c := range clients {
		if err := c.NotifyChange(ctx, path); err != nil {
			debug.Log("[lsp] notify change error for %s: %v", c.Name(), err)

			continue
		}

		c.WaitForDiagnostics(ctx, diagnosticWaitTimeout)
	}
}

// DidOpen opens the file in all LSP servers that handle it.
// This is the entry point for Read (no change notification, no diagnostic wait).
func (m *Manager) DidOpen(ctx context.Context, path string) {
	path = m.ResolvePath(path)
	m.Start(ctx, path)

	m.mu.Lock()
	clients := m.clientsForFile(path)
	m.mu.Unlock()

	for _, c := range clients {
		if err := c.OpenFileOnDemand(ctx, path); err != nil {
			debug.Log("[lsp] didOpen error for %s: %v", c.Name(), err)
		}
	}
}

// FormatDiagnostics returns formatted diagnostics from all clients.
// If path is non-empty, only diagnostics for that file are included.
// Resolves relative paths to absolute for URI matching.
// Returns empty string if there are no diagnostics.
func (m *Manager) FormatDiagnostics(path string) string {
	if path != "" {
		path = m.ResolvePath(path)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var lines []string

	for _, client := range m.clients {
		for uri, diags := range client.Diagnostics() {
			filePath, err := uri.Path()
			if err != nil {
				continue
			}

			fp := file.New(filePath)
			if !fp.Exists() {
				client.ClearDiagnostic(uri)

				continue
			}

			if path != "" && filePath != path {
				continue
			}

			for _, d := range diags {
				severity := diagnosticSeverity(d.Severity)

				lines = append(lines, fmt.Sprintf("%s:%d:%d %s: %s",
					fp.Base(),
					d.Range.Start.Line+1,
					d.Range.Start.Character+1,
					severity, d.Message))
			}
		}
	}

	sort.Strings(lines)

	return strings.Join(lines, "\n")
}

// AllDiagnostics starts servers for the path, notifies changes, and returns formatted diagnostics.
// This is the entry point for the Diagnostics tool.
func (m *Manager) AllDiagnostics(ctx context.Context, path string) string {
	if path != "" {
		path = m.ResolvePath(path)
		m.NotifyChange(ctx, path)
	}

	return m.FormatDiagnostics(path)
}

// StopAll gracefully shuts down all running LSP servers.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for name, client := range m.clients {
		debug.Log("[lsp] stopping %s", name)
		client.Shutdown(ctx)
	}

	m.clients = make(map[string]*Client)
}

// RestartAll stops and restarts all previously configured servers.
func (m *Manager) RestartAll(ctx context.Context) {
	m.StopAll()

	// Re-initialization will happen lazily on next file touch.
	// Force-start servers that have root markers present.
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, server := range m.config.Servers {
		if server.Disabled {
			continue
		}

		if !m.HasRootMarkers(server) {
			continue
		}

		debug.Log("[lsp] restarting %s", name)

		rootURI := "file://" + m.workDir

		client, err := NewClient(ctx, name, server, rootURI)
		if err != nil {
			debug.Log("[lsp] failed to restart %s: %v", name, err)

			continue
		}

		m.clients[name] = client
		debug.Log("[lsp] %s ready", name)
	}
}

// RestartServer stops and restarts a single named server.
// Returns an error if the server name is not in the configuration.
func (m *Manager) RestartServer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	server, ok := m.config.Servers[name]
	if !ok {
		return fmt.Errorf("unknown LSP server %q", name)
	}

	if server.Disabled {
		return fmt.Errorf("LSP server %q is disabled", name)
	}

	// Stop existing client if running.
	if client, running := m.clients[name]; running {
		debug.Log("[lsp] stopping %s", name)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client.Shutdown(shutdownCtx)

		delete(m.clients, name)
	}

	debug.Log("[lsp] restarting %s", name)

	rootURI := "file://" + m.workDir

	client, err := NewClient(ctx, name, server, rootURI)
	if err != nil {
		return fmt.Errorf("restarting LSP server %q: %w", name, err)
	}

	m.clients[name] = client
	debug.Log("[lsp] %s ready", name)

	return nil
}

// ServerNames returns the names of all configured servers.
func (m *Manager) ServerNames() []string {
	names := make([]string, 0, len(m.config.Servers))
	for name := range m.config.Servers {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// ServerCount returns the number of running LSP servers.
func (m *Manager) ServerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.clients)
}

// clientsForFile returns all running clients that handle the given file path.
// Must be called with m.mu held.
func (m *Manager) clientsForFile(path string) []*Client {
	var result []*Client

	for _, c := range m.clients {
		if c.HandlesFile(path) {
			result = append(result, c)
		}
	}

	return result
}

// HandlesFile checks if a server configuration matches the given file path
// by extension or detected language.
func (m *Manager) HandlesFile(server Server, path string) bool {
	if len(server.FileTypes) == 0 {
		return true
	}

	ext := fileExtension(path)
	lang := string(lsp.DetectLanguage(path))

	for _, ft := range server.FileTypes {
		ft = strings.TrimPrefix(ft, ".")
		if strings.EqualFold(ft, ext) || strings.EqualFold(ft, lang) {
			return true
		}
	}

	return false
}

// HasRootMarkers checks if at least one root marker is present in the working directory.
func (m *Manager) HasRootMarkers(server Server) bool {
	if len(server.RootMarkers) == 0 {
		return true
	}

	for _, marker := range server.RootMarkers {
		matches, _ := folder.New(m.workDir).Glob(marker)
		if len(matches) > 0 {
			return true
		}
	}

	return false
}

// ResolvePath converts a relative path to an absolute path using the working directory.
// Returns the path unchanged if it's already absolute or if resolution fails.
func (m *Manager) ResolvePath(path string) string {
	f := file.New(path)
	if f.IsAbs() {
		return path
	}

	return file.New(m.workDir, path).Path()
}

// diagnosticSeverity converts a protocol severity to a human-readable string.
func diagnosticSeverity(s protocol.DiagnosticSeverity) string {
	switch s {
	case protocol.SeverityError:
		return "error"
	case protocol.SeverityWarning:
		return "warning"
	case protocol.SeverityInformation:
		return "info"
	case protocol.SeverityHint:
		return "hint"
	default:
		return "error"
	}
}
