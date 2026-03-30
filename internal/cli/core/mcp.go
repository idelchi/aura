package core

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/spinner"
)

// connectServers builds the enabled server map (with optional name whitelist)
// and connects in parallel. Returns nil if no servers matched.
func connectServers(
	ctx context.Context,
	mcps config.StringCollection[mcp.Server],
	only []string,
	onProgress func(string),
) []mcp.Result {
	allowed := make(map[string]bool, len(only))
	for _, name := range only {
		allowed[name] = true
	}

	servers := make(map[string]mcp.Server)

	for _, name := range mcps.Names() {
		if len(allowed) > 0 && !allowed[name] {
			continue
		}

		if s := mcps[name]; s.IsEnabled() {
			servers[name] = s
		}
	}

	if len(servers) == 0 {
		return nil
	}

	return mcp.ConnectAll(ctx, servers, onProgress)
}

// ConnectMCP connects to MCP servers in parallel and registers tools on the assistant.
// Blocks until all servers are connected or confirmed unreachable.
func ConnectMCP(ctx context.Context, asst *assistant.Assistant, events chan<- ui.Event) {
	mcpInclude, mcpExclude := asst.Cfg().MCPFilter(asst.Runtime())
	mcps := config.FilterMCPs(asst.Cfg().MCPs, mcpInclude, mcpExclude)

	events <- ui.SpinnerMessage{Text: "Connecting to MCP servers..."}

	results := connectServers(ctx, mcps, nil, func(name string) {
		events <- ui.SpinnerMessage{Text: fmt.Sprintf("Connecting to %s...", name)}
	})

	var sessions []*mcp.Session

	for _, r := range results {
		if r.Error != nil {
			events <- ui.CommandResult{
				Command: "mcp",
				Error:   fmt.Errorf("%s", r.StatusDisplay()),
			}

			continue
		}

		sessions = append(sessions, r.Session)
	}

	asst.RegisterMCPTools(sessions)

	// Clear MCP spinner text so it doesn't bleed into LLM wait spinner
	events <- ui.SpinnerMessage{}
}

// ConnectMCPServers connects to enabled MCP servers and returns sessions + warnings.
// When only is non-empty, only servers in the whitelist are connected.
// Used by the tools command for standalone MCP connection.
func ConnectMCPServers(
	ctx context.Context,
	mcps config.StringCollection[mcp.Server],
	only []string,
	interactive bool,
) ([]*mcp.Session, []string) {
	var s *spinner.Spinner

	if interactive {
		s = spinner.New("Connecting to MCP servers...")
		s.Start()
	}

	results := connectServers(ctx, mcps, only, func(name string) {
		if s != nil {
			s.Update(fmt.Sprintf("Connecting to %s...", name))
		}
	})

	if s != nil {
		s.Stop()
	}

	var (
		sessions []*mcp.Session
		warnings []string
	)

	for _, r := range results {
		if r.Error != nil {
			warnings = append(warnings, r.StatusDisplay())

			continue
		}

		sessions = append(sessions, r.Session)
	}

	return sessions, warnings
}
