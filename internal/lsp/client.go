package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/x/powernap/pkg/lsp"
	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/godyl/pkg/path/file"
)

// ServerState represents the lifecycle state of an LSP server.
type ServerState int

const (
	StateStarting ServerState = iota
	StateReady
	StateError
	StateStopped
)

// String returns a human-readable state name.
func (s ServerState) String() string {
	switch s {
	case StateStarting:
		return "starting"
	case StateReady:
		return "ready"
	case StateError:
		return "error"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// FileInfo tracks an open file's version for incremental change notifications.
type FileInfo struct {
	URI     protocol.DocumentURI
	Version int32
}

// Client wraps a powernap LSP client with diagnostic caching and file tracking.
type Client struct {
	client *lsp.Client
	name   string
	config Server

	// State machine
	state atomic.Value // ServerState

	// Diagnostic cache — written by notification handler, read by FormatDiagnostics
	diagMu      sync.Mutex
	diagnostics map[protocol.DocumentURI][]protocol.Diagnostic
	diagVersion uint64

	// Open file tracking
	fileMu    sync.Mutex
	openFiles map[string]*FileInfo // URI string → FileInfo
}

// NewClient creates a powernap LSP client, registers the diagnostic handler,
// and initializes the connection. Returns an error if initialization fails.
func NewClient(ctx context.Context, name string, server Server, rootURI string) (*Client, error) {
	pnClient, err := lsp.NewClient(lsp.ClientConfig{
		Command:     server.Command,
		Args:        server.Args,
		RootURI:     rootURI,
		Settings:    server.Settings,
		InitOptions: server.InitOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("creating LSP client %q: %w", name, err)
	}

	c := &Client{
		client:      pnClient,
		name:        name,
		config:      server,
		diagnostics: make(map[protocol.DocumentURI][]protocol.Diagnostic),
		openFiles:   make(map[string]*FileInfo),
	}

	c.state.Store(StateStarting)

	// Register diagnostic notification handler before initialize
	pnClient.RegisterNotificationHandler("textDocument/publishDiagnostics",
		func(_ context.Context, _ string, params json.RawMessage) {
			var dp protocol.PublishDiagnosticsParams
			if err := json.Unmarshal(params, &dp); err != nil {
				debug.Log("[lsp] diagnostic unmarshal: %v", err)

				return
			}

			c.diagMu.Lock()
			c.diagnostics[dp.URI] = dp.Diagnostics
			c.diagVersion++
			c.diagMu.Unlock()
		},
	)

	// Initialize with timeout
	timeout := time.Duration(server.TimeoutOrDefault()) * time.Second

	initCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := pnClient.Initialize(initCtx, false); err != nil {
		c.state.Store(StateError)
		pnClient.Kill()

		return nil, fmt.Errorf("initializing LSP server %q: %w", name, err)
	}

	c.state.Store(StateReady)

	return c, nil
}

// Name returns the server name.
func (c *Client) Name() string { return c.name }

// State returns the current server state.
func (c *Client) State() ServerState { return c.state.Load().(ServerState) }

// HandlesFile checks whether this client handles the given file path
// based on configured file types and language detection.
func (c *Client) HandlesFile(path string) bool {
	if len(c.config.FileTypes) == 0 {
		return true
	}

	ext := fileExtension(path)
	lang := string(lsp.DetectLanguage(path))

	for _, ft := range c.config.FileTypes {
		ft = strings.TrimPrefix(ft, ".")
		if strings.EqualFold(ft, ext) || strings.EqualFold(ft, lang) {
			return true
		}
	}

	return false
}

// OpenFileOnDemand opens the file in the LSP server if not already open.
func (c *Client) OpenFileOnDemand(ctx context.Context, path string) error {
	uri := string(protocol.URIFromPath(path))

	c.fileMu.Lock()
	defer c.fileMu.Unlock()

	if _, open := c.openFiles[uri]; open {
		return nil
	}

	content, err := file.New(path).Read()
	if err != nil {
		return fmt.Errorf("reading file for LSP open: %w", err)
	}

	lang := string(lsp.DetectLanguage(path))

	if err := c.client.NotifyDidOpenTextDocument(ctx, uri, lang, 1, string(content)); err != nil {
		return fmt.Errorf("LSP didOpen: %w", err)
	}

	c.openFiles[uri] = &FileInfo{URI: protocol.DocumentURI(uri), Version: 1}

	return nil
}

// NotifyChange reads the file from disk and sends a full-content change notification.
func (c *Client) NotifyChange(ctx context.Context, path string) error {
	uri := string(protocol.URIFromPath(path))

	content, err := file.New(path).Read()
	if err != nil {
		return fmt.Errorf("reading file for LSP change: %w", err)
	}

	c.fileMu.Lock()
	fi, open := c.openFiles[uri]

	if !open {
		fi = &FileInfo{URI: protocol.DocumentURI(uri), Version: 0}
		c.openFiles[uri] = fi
	}

	fi.Version++

	version := int(fi.Version)
	c.fileMu.Unlock()

	changes := []protocol.TextDocumentContentChangeEvent{
		{Value: protocol.TextDocumentContentChangeWholeDocument{Text: string(content)}},
	}

	if !open {
		// File wasn't open yet — send didOpen first
		lang := string(lsp.DetectLanguage(path))
		if err := c.client.NotifyDidOpenTextDocument(ctx, uri, lang, version, string(content)); err != nil {
			return fmt.Errorf("LSP didOpen: %w", err)
		}

		return nil
	}

	return c.client.NotifyDidChangeTextDocument(ctx, uri, version, changes)
}

// WaitForDiagnostics polls until the diagnostic version changes or the timeout expires.
func (c *Client) WaitForDiagnostics(ctx context.Context, timeout time.Duration) {
	c.diagMu.Lock()
	startVersion := c.diagVersion
	c.diagMu.Unlock()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case <-ticker.C:
			c.diagMu.Lock()
			changed := c.diagVersion != startVersion
			c.diagMu.Unlock()

			if changed {
				return
			}
		}
	}
}

// Diagnostics returns a snapshot of the cached diagnostics.
func (c *Client) Diagnostics() map[protocol.DocumentURI][]protocol.Diagnostic {
	c.diagMu.Lock()
	defer c.diagMu.Unlock()

	snapshot := make(map[protocol.DocumentURI][]protocol.Diagnostic, len(c.diagnostics))
	maps.Copy(snapshot, c.diagnostics)

	return snapshot
}

// ClearDiagnostic removes cached diagnostics for a single URI.
func (c *Client) ClearDiagnostic(uri protocol.DocumentURI) {
	c.diagMu.Lock()
	delete(c.diagnostics, uri)
	c.diagMu.Unlock()
}

// Shutdown gracefully shuts down the LSP server.
func (c *Client) Shutdown(ctx context.Context) {
	c.state.Store(StateStopped)

	// Best-effort shutdown — ignore errors from already-dead servers
	_ = c.client.Shutdown(ctx)
	_ = c.client.Exit()
}

// Kill forcefully terminates the LSP server.
func (c *Client) Kill() {
	c.state.Store(StateStopped)
	c.client.Kill()
}

// fileExtension returns the extension without the leading dot.
func fileExtension(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i+1:]
		}

		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}

	return ""
}
