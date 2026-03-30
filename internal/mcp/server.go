package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/idelchi/godyl/pkg/env"
)

// Server represents an MCP server configuration.
type Server struct {
	Disabled  bool              `yaml:"disabled"`                                   // true = skip connection (default false = enabled)
	Deferred  bool              `yaml:"deferred"`                                   // true = all tools from this server are deferred
	Type      string            `validate:"omitempty,oneof=http stdio" yaml:"type"` // "http" or "stdio"
	URL       string            `yaml:"url"`                                        // For HTTP transport
	Command   string            `yaml:"command"`                                    // For STDIO transport
	Args      []string          `yaml:"args"`                                       // For STDIO transport
	Env       env.Env           `yaml:"env"`                                        // Environment variables
	Headers   map[string]string `yaml:"headers"`                                    // For HTTP transport
	Timeout   time.Duration     `yaml:"timeout"`                                    // Connection timeout
	Condition string            `yaml:"condition"`                                  // condition expression for conditional inclusion
	Source    string            `yaml:"-"`                                          // file path this server was loaded from
}

// IsEnabled reports whether this server should be connected at startup.
func (s Server) IsEnabled() bool {
	return !s.Disabled
}

// Connect creates and initializes an MCP client session.
func (s Server) Connect(ctx context.Context) (client.MCPClient, error) {
	var (
		c   client.MCPClient
		err error
	)

	switch s.Type {
	case "http":
		c, err = s.httpClient()
	case "stdio", "":
		c, err = s.stdioClient()
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", s.Type)
	}

	if err != nil {
		return nil, err
	}

	// Initialize the connection
	_, err = c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "aura",
				Version: "0.0.0",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initializing MCP connection: %w", err)
	}

	return c, nil
}

func (s Server) httpClient() (client.MCPClient, error) {
	var opts []transport.StreamableHTTPCOption

	if len(s.Headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(s.Headers))
	}

	return client.NewStreamableHttpClient(s.URL, opts...)
}

func (s Server) stdioClient() (client.MCPClient, error) {
	e := s.Env

	return client.NewStdioMCPClient(s.Command, e.AsSlice(), s.Args...)
}

// Display returns a human-readable summary of the server's connection target.
// For HTTP: the URL. For STDIO: command + args.
func (s Server) Display() string {
	typ := s.Type
	if typ == "" {
		typ = "stdio"
	}

	switch typ {
	case "http":
		return "url: " + s.URL
	default:
		if len(s.Args) > 0 {
			return "command: " + s.Command + " " + strings.Join(s.Args, " ")
		}

		return "command: " + s.Command
	}
}

// EffectiveType returns the transport type, defaulting to "stdio".
func (s Server) EffectiveType() string {
	if s.Type == "" {
		return "stdio"
	}

	return s.Type
}

// Expanded returns a copy with environment variables expanded.
func (s Server) Expanded() Server {
	expanded := s

	expanded.Command = os.ExpandEnv(s.Command)
	expanded.URL = os.ExpandEnv(s.URL)

	expanded.Args = make([]string, len(s.Args))
	for i, arg := range s.Args {
		expanded.Args[i] = os.ExpandEnv(arg)
	}

	expanded.Env = make(env.Env)
	for k, v := range s.Env {
		expanded.Env[k] = os.ExpandEnv(v)
	}

	expanded.Headers = make(map[string]string)
	for k, v := range s.Headers {
		expanded.Headers[k] = os.ExpandEnv(v)
	}

	if expanded.Type == "" {
		expanded.Type = "stdio"
	}

	return expanded
}
