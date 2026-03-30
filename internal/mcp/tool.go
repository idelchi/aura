package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/idelchi/aura/pkg/llm/tool"
)

// Tool wraps an MCP tool to implement the tool.Tool interface.
type Tool struct {
	tool.Base

	serverName string
	mcpTool    mcplib.Tool
	client     client.MCPClient
}

// NewTool creates a Tool wrapper.
func NewTool(serverName string, mcpTool mcplib.Tool, client client.MCPClient) *Tool {
	return &Tool{
		serverName: serverName,
		mcpTool:    mcpTool,
		client:     client,
	}
}

// Name returns the prefixed tool name.
func (t *Tool) Name() string {
	return MakeName(t.serverName, t.mcpTool.Name)
}

// Description returns the MCP tool's description.
func (t *Tool) Description() string {
	return t.mcpTool.Description
}

// Usage returns usage instructions.
func (t *Tool) Usage() string {
	return "MCP tool from server: " + t.serverName
}

// Examples returns empty string (MCP tools don't provide examples).
func (t *Tool) Examples() string {
	return ""
}

// Schema converts MCP schema to tool.Schema.
func (t *Tool) Schema() tool.Schema {
	return convertSchema(t.Name(), t.mcpTool)
}

// Execute calls the MCP tool on the remote server.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	result, err := t.client.CallTool(ctx, mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Name:      t.mcpTool.Name,
			Arguments: args,
		},
	})
	if err != nil {
		return "", fmt.Errorf("mcp tool call: %w", err)
	}

	return formatResult(result.Content), nil
}

func formatResult(content []mcplib.Content) string {
	var parts []string

	for _, c := range content {
		switch v := c.(type) {
		case mcplib.TextContent:
			parts = append(parts, v.Text)
		case *mcplib.TextContent:
			parts = append(parts, v.Text)
		default:
			if b, err := json.Marshal(v); err == nil {
				parts = append(parts, string(b))
			}
		}
	}

	return strings.Join(parts, "\n")
}

// Sandboxable returns false because MCP tools execute on remote servers.
func (t *Tool) Sandboxable() bool {
	return false
}

// MakeName constructs a prefixed MCP tool name from server and tool names.
func MakeName(server, tool string) string {
	return fmt.Sprintf("mcp__%s__%s", server, tool)
}

// ExtractServer parses an MCP tool name (mcp__server__tool) and returns
// the server name and short tool name. Returns ok=false for non-MCP names.
func ExtractServer(name string) (server, shortName string, ok bool) {
	parts := strings.SplitN(name, "__", 3)
	if len(parts) != 3 || parts[0] != "mcp" {
		return "", "", false
	}

	return parts[1], parts[2], true
}
