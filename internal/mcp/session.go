package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/idelchi/aura/internal/debug"
)

// Session represents a connected MCP server session.
type Session struct {
	Name     string           // Server name for tool prefixing
	Client   client.MCPClient // Active MCP client
	MCPTools []mcplib.Tool    // Discovered MCP tools
}

// NewSession connects to a server and discovers its tools.
func NewSession(ctx context.Context, name string, server Server) (*Session, error) {
	debug.Log("[mcp] %s: connecting...", name)

	cfg := server.Expanded()

	c, err := cfg.Connect(ctx)
	if err != nil {
		debug.Log("[mcp] %s: connection failed: %v", name, err)

		return nil, err
	}

	// List available tools
	result, err := c.ListTools(ctx, mcplib.ListToolsRequest{})
	if err != nil {
		debug.Log("[mcp] %s: tool discovery failed: %v", name, err)

		return nil, fmt.Errorf("listing tools: %w", err)
	}

	debug.Log("[mcp] %s: connected, discovered %d tools", name, len(result.Tools))

	return &Session{
		Name:     name,
		Client:   c,
		MCPTools: result.Tools,
	}, nil
}

// Tools returns wrapped MCP tools implementing tool.Tool interface.
func (s *Session) Tools() []*Tool {
	tools := make([]*Tool, len(s.MCPTools))
	for i, t := range s.MCPTools {
		tools[i] = NewTool(s.Name, t, s.Client)
	}

	return tools
}

// Close disconnects the MCP client.
func (s *Session) Close() error {
	if s.Client != nil {
		return s.Client.Close()
	}

	return nil
}
