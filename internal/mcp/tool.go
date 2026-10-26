package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// Tool wraps an MCP tool to implement the tool.Tool interface.
type Tool struct {
	tool.Base

	serverName string
	mcpTool    mcplib.Tool
	client     *client.Client
	calls      *atomic.Int64 // request ID sequence shared by the session's tools
}

// NewTool creates a Tool wrapper.
func NewTool(serverName string, mcpTool mcplib.Tool, client *client.Client, calls *atomic.Int64) *Tool {
	return &Tool{
		serverName: serverName,
		mcpTool:    mcpTool,
		client:     client,
		calls:      calls,
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
//
// The request is sent with an ID owned by Aura so that, when ctx is cancelled (run
// cancellation or shutdown), the server receives notifications/cancelled for exactly
// this request and can stop its work; the MCP client library does not send it.
// A result with isError is returned as an error.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	transporter := t.client.GetTransport()
	id := mcplib.NewRequestId(fmt.Sprintf("aura-%d", t.calls.Add(1)))

	response, err := transporter.SendRequest(ctx, transport.JSONRPCRequest{
		JSONRPC: mcplib.JSONRPC_VERSION,
		ID:      id,
		Method:  string(mcplib.MethodToolsCall),
		Params:  mcplib.CallToolParams{Name: t.mcpTool.Name, Arguments: args},
	})
	if err != nil {
		if ctx.Err() != nil {
			t.cancel(id, context.Cause(ctx))
		}

		return "", fmt.Errorf("mcp tool call: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("mcp tool call: %w", response.Error.AsError())
	}

	result, err := mcplib.ParseCallToolResult(&response.Result)
	if err != nil {
		return "", fmt.Errorf("mcp tool call: %w", err)
	}

	output := formatResult(result.Content)

	// A tool-level failure is an error result, not a successful call with error text.
	if result.IsError {
		if output == "" {
			output = "tool reported an error without content"
		}

		return "", errors.New(output)
	}

	return output, nil
}

// cancel tells the server to abandon a request (MCP notifications/cancelled).
func (t *Tool) cancel(id mcplib.RequestId, cause error) {
	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()

	reason := "cancelled by the client"
	if cause != nil {
		reason = cause.Error()
	}

	err := t.client.GetTransport().SendNotification(ctx, mcplib.JSONRPCNotification{
		JSONRPC: mcplib.JSONRPC_VERSION,
		Notification: mcplib.Notification{
			Method: "notifications/cancelled",
			Params: mcplib.NotificationParams{AdditionalFields: map[string]any{"requestId": id.Value(), "reason": reason}},
		},
	})
	if err != nil {
		debug.Log("[mcp] %s: sending cancellation of %v: %v", t.serverName, id.Value(), err)
	}
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
