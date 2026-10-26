package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/idelchi/aura/pkg/sandbox"
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
	Required  bool              `yaml:"required"`                                   // a failed connection fails startup
	Sandbox   bool              `yaml:"sandbox"`                                    // stdio only: confine with the session's Landlock restrictions
	Source    string            `yaml:"-"`                                          // file path this server was loaded from
}

// Validate rejects settings that cannot be honoured.
func (s Server) Validate() error {
	if s.Sandbox && s.EffectiveType() != "stdio" {
		return errors.New("sandbox applies only to stdio servers: a remote http server cannot be confined locally")
	}

	return nil
}

// IsEnabled reports whether this server should be connected at startup.
func (s Server) IsEnabled() bool {
	return !s.Disabled
}

// Connect creates and initializes an MCP client session. A sandboxed stdio server is
// launched under confinement; its process is returned so the session can reap its group.
func (s Server) Connect(ctx context.Context, confinement Confinement, stderr *Stderr) (client.MCPClient, *exec.Cmd, error) {
	var (
		c       *client.Client
		process *exec.Cmd
		err     error
	)

	switch s.Type {
	case "http":
		c, err = s.httpClient()
	case "stdio", "":
		c, process, err = s.stdioClient(confinement)
		if err == nil {
			if r, ok := client.GetStderr(c); ok {
				stderr.Drain(r)
			}
		}
	default:
		return nil, nil, fmt.Errorf("unsupported transport type: %s", s.Type)
	}

	if err != nil {
		return nil, nil, err
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
		c.Close()

		return nil, nil, fmt.Errorf("initializing MCP connection: %w", err)
	}

	return c, process, nil
}

func (s Server) httpClient() (*client.Client, error) {
	var opts []transport.StreamableHTTPCOption

	if len(s.Headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(s.Headers))
	}

	return client.NewStreamableHttpClient(s.URL, opts...)
}

func (s Server) stdioClient(confinement Confinement) (*client.Client, *exec.Cmd, error) {
	e := s.Env

	if !s.Sandbox {
		c, err := client.NewStdioMCPClient(s.Command, e.AsSlice(), s.Args...)

		return c, nil, err
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("locating the aura executable for sandboxing: %w", err)
	}

	var process *exec.Cmd

	launch := func(_ context.Context, command string, env []string, args []string) (*exec.Cmd, error) {
		cmd, err := sandbox.Command(executable, confinement.WorkDir, confinement.Spec(), command, args...)
		if err != nil {
			return nil, err
		}

		cmd.Env = append(os.Environ(), env...)
		process = cmd

		return cmd, nil
	}

	c, err := client.NewStdioMCPClientWithOptions(s.Command, e.AsSlice(), s.Args, transport.WithCommandFunc(launch))

	return c, process, err
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

// Expanded returns a copy with $VAR and ${VAR} references expanded.
// A reference to an unset variable is an error (an empty but set variable is allowed),
// so a missing token or path never silently becomes an empty string.
func (s Server) Expanded() (Server, error) {
	var missing []string

	expand := func(value string) string {
		return os.Expand(value, func(name string) string {
			v, ok := os.LookupEnv(name)
			if !ok && !slices.Contains(missing, name) {
				missing = append(missing, name)
			}

			return v
		})
	}

	expanded := s

	expanded.Command = expand(s.Command)
	expanded.URL = expand(s.URL)

	expanded.Args = make([]string, len(s.Args))
	for i, arg := range s.Args {
		expanded.Args[i] = expand(arg)
	}

	expanded.Env = make(env.Env)
	for k, v := range s.Env {
		expanded.Env[k] = expand(v)
	}

	expanded.Headers = make(map[string]string)
	for k, v := range s.Headers {
		expanded.Headers[k] = expand(v)
	}

	if expanded.Type == "" {
		expanded.Type = "stdio"
	}

	if len(missing) > 0 {
		return Server{}, fmt.Errorf("unset environment variables: %s", strings.Join(missing, ", "))
	}

	return expanded, nil
}
