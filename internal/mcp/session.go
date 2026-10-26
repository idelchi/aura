package mcp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/sandbox"
)

// Session represents a connected MCP server session.
type Session struct {
	Name      string           // Server name for tool prefixing
	Client    client.MCPClient // Active MCP client
	MCPTools  []mcplib.Tool    // Discovered MCP tools
	Required  bool             // configured as required
	Sandboxed bool             // launched under Landlock confinement

	process *exec.Cmd // confined launcher process, whose group is reaped on Close
}

// NewSession connects to a server and discovers its tools.
func NewSession(ctx context.Context, name string, server Server, confinement Confinement) (*Session, error) {
	debug.Log("[mcp] %s: connecting (sandbox=%t)...", name, server.Sandbox)

	cfg, err := server.Expanded()
	if err != nil {
		return nil, err
	}

	stderr := &Stderr{name: name}

	c, process, err := cfg.Connect(ctx, confinement, stderr)
	if err != nil {
		debug.Log("[mcp] %s: connection failed: %v", name, err)

		return nil, withStderr(err, stderr)
	}

	sess := &Session{Name: name, Client: c, Required: server.Required, Sandboxed: server.Sandbox, process: process}

	// List available tools
	result, err := c.ListTools(ctx, mcplib.ListToolsRequest{})
	if err != nil {
		debug.Log("[mcp] %s: tool discovery failed: %v", name, err)
		sess.Close()

		return nil, withStderr(fmt.Errorf("listing tools: %w", err), stderr)
	}

	debug.Log("[mcp] %s: connected, discovered %d tools", name, len(result.Tools))

	sess.MCPTools = result.Tools

	return sess, nil
}

// withStderr appends the server's recent stderr to a connection error.
func withStderr(err error, stderr *Stderr) error {
	// Give the drain goroutine a moment to read what an exiting server wrote.
	time.Sleep(50 * time.Millisecond)

	if tail := stderr.Tail(); tail != "" {
		return fmt.Errorf("%w\nserver stderr:\n%s", err, tail)
	}

	return err
}

// Tools returns wrapped MCP tools implementing tool.Tool interface.
func (s *Session) Tools() []*Tool {
	tools := make([]*Tool, len(s.MCPTools))
	for i, t := range s.MCPTools {
		tools[i] = NewTool(s.Name, t, s.Client)
	}

	return tools
}

// Close disconnects the MCP client. For a confined server, the whole process group
// (including any children the server started) is killed after the client closes.
func (s *Session) Close() error {
	var err error

	if s.Client != nil {
		err = s.Client.Close()
	}

	if s.process != nil && s.process.Process != nil {
		if killErr := sandbox.KillGroup(s.process.Process.Pid); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
			debug.Log("[mcp] %s: killing process group: %v", s.Name, killErr)
		}
	}

	return err
}
