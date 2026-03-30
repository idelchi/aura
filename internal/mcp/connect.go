package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/idelchi/aura/internal/debug"
)

// Result holds the outcome of connecting to a single MCP server.
type Result struct {
	Name    string   // Server name
	Session *Session // non-nil on success
	Error   error    // non-nil on failure
}

// StatusDisplay returns a formatted connection status string.
func (r Result) StatusDisplay() string {
	if r.Error != nil {
		return fmt.Sprintf("%s: %v", r.Name, r.Error)
	}

	return fmt.Sprintf("%s: connected (%d tools)", r.Name, len(r.Session.MCPTools))
}

// ToolNames returns the prefixed tool names from a successful connection.
// Returns nil if the connection failed.
func (r Result) ToolNames() []string {
	if r.Session == nil {
		return nil
	}

	names := make([]string, len(r.Session.MCPTools))
	for i, t := range r.Session.MCPTools {
		names[i] = MakeName(r.Name, t.Name)
	}

	return names
}

// ConnectAll connects to multiple MCP servers in parallel.
// progress is called (if non-nil) when each server connection starts.
// Returns a result for every server (both successes and failures).
func ConnectAll(ctx context.Context, servers map[string]Server, progress func(name string)) []Result {
	debug.Log("[mcp] connecting to %d servers", len(servers))

	var (
		g   errgroup.Group
		mu  sync.Mutex
		out []Result
	)

	g.SetLimit(16)

	for name, server := range servers {
		g.Go(func() error {
			if progress != nil {
				progress(name)
			}

			timeout := server.Timeout
			if timeout == 0 {
				timeout = 10 * time.Second
			}

			connCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			sess, err := NewSession(connCtx, name, server)

			mu.Lock()

			out = append(out, Result{Name: name, Session: sess, Error: err})

			mu.Unlock()

			return nil
		})
	}

	g.Wait()

	var ok, failed int

	for _, r := range out {
		if r.Error != nil {
			failed++
		} else {
			ok++
		}
	}

	debug.Log("[mcp] done: %d connected, %d failed", ok, failed)

	return out
}
