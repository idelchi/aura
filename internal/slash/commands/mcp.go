package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/internal/slash"
)

// MCP creates the /mcp command to list connected MCP servers or reconnect failed ones.
func MCP() slash.Command {
	return slash.Command{
		Name:        "/mcp",
		Description: "List connected MCP servers or reconnect failed ones",
		Hints:       "[reconnect [server]]",
		Category:    "tools",
		Execute: func(ctx context.Context, c slash.Context, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "reconnect" {
				return mcpReconnect(ctx, c, args[1:]...)
			}

			return mcpList(c)
		},
	}
}

func mcpList(c slash.Context) (string, error) {
	if len(c.MCPSessions()) == 0 {
		return "No MCP servers connected", nil
	}

	var b strings.Builder
	b.WriteString("MCP servers:\n")

	for _, s := range c.MCPSessions() {
		tools := s.Tools()
		fmt.Fprintf(&b, "- %s (%d tools)\n", s.Name, len(tools))

		for _, t := range tools {
			b.WriteString("    " + t.Name() + "\n")
		}
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

func mcpReconnect(ctx context.Context, c slash.Context, args ...string) (string, error) {
	cfg := c.Cfg()
	include, exclude := cfg.MCPFilter(c.Runtime())
	mcps := config.FilterMCPs(cfg.MCPs, include, exclude)

	// Build set of already-connected server names.
	connected := make(map[string]bool)

	for _, s := range c.MCPSessions() {
		connected[s.Name] = true
	}

	// If a specific server was requested, validate it first.
	var only string

	if len(args) > 0 && args[0] != "" {
		only = args[0]
		if mcps.Get(only) == nil {
			return fmt.Sprintf("Server %q not found in config", only), nil
		}

		if connected[strings.ToLower(only)] {
			return fmt.Sprintf("Server %q is already connected", only), nil
		}
	}

	// Collect unconnected, enabled servers.
	servers := make(map[string]mcp.Server)

	for _, name := range mcps.Names() {
		if only != "" && !strings.EqualFold(name, only) {
			continue
		}

		if connected[name] {
			continue
		}

		if s := mcps[name]; s.IsEnabled() {
			servers[name] = s
		}
	}

	if len(servers) == 0 {
		return "All enabled servers are already connected", nil
	}

	results := mcp.ConnectAll(ctx, servers, nil)

	var b strings.Builder

	for _, r := range results {
		if r.Error != nil {
			fmt.Fprintf(&b, "x %s: %v\n", r.Name, r.Error)
		} else {
			if err := c.RegisterMCPSession(r.Session); err != nil {
				fmt.Fprintf(&b, "x %s: connected but state rebuild failed: %v\n", r.Name, err)

				continue
			}

			fmt.Fprintf(&b, "+ %s: connected (%d tools)\n", r.Name, len(r.Session.Tools()))
		}
	}

	return strings.TrimRight(b.String(), "\n"), nil
}
