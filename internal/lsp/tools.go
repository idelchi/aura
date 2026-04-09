package lsp

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
)

// DiagnosticsInputs defines the parameters for the Diagnostics tool.
type DiagnosticsInputs struct {
	Path string `json:"path,omitempty" jsonschema:"description=File path to get diagnostics for. Omit for project-wide diagnostics."`
}

// DiagnosticsTool returns LSP diagnostics for a file or the whole project.
type DiagnosticsTool struct {
	tool.Base

	manager *Manager
}

// NewDiagnosticsTool creates a Diagnostics tool backed by the given manager.
func NewDiagnosticsTool(manager *Manager) *DiagnosticsTool {
	return &DiagnosticsTool{
		Base: tool.Base{
			Text: tool.Text{
				Description: `Get compiler diagnostics (errors, warnings) from running LSP servers.`,
				Usage: heredoc.Doc(`
					Returns diagnostics from all active LSP servers.
					If path is provided, only diagnostics for that file are returned.
					If path is omitted, returns diagnostics for all open files.
					LSP servers are lazily started when a file path is provided.
				`),
				Examples: heredoc.Doc(`
					{"path": "internal/server/handler.go"}
					{}
				`),
			},
		},
		manager: manager,
	}
}

func (t *DiagnosticsTool) Name() string { return "Diagnostics" }

func (t *DiagnosticsTool) Schema() tool.Schema {
	return tool.GenerateSchema[DiagnosticsInputs](t)
}

func (t *DiagnosticsTool) Available() bool {
	return t.manager != nil
}

// Sandboxable returns false because the tool communicates with in-process LSP servers
// via the manager pointer, which does not exist in the sandboxed child process.
func (t *DiagnosticsTool) Sandboxable() bool { return false }

func (t *DiagnosticsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[DiagnosticsInputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	diags := t.manager.AllDiagnostics(ctx, params.Path)
	if diags == "" {
		return "No diagnostics.", nil
	}

	return diags, nil
}

// RestartInputs defines the parameters for the LspRestart tool.
type RestartInputs struct {
	Server string `json:"server,omitempty" jsonschema:"description=Server name to restart. Omit to restart all servers."`
}

// RestartTool restarts LSP servers.
type RestartTool struct {
	tool.Base

	manager *Manager
}

// NewRestartTool creates an LspRestart tool backed by the given manager.
func NewRestartTool(manager *Manager) *RestartTool {
	return &RestartTool{
		Base: tool.Base{
			Text: tool.Text{
				Description: `Restart LSP servers. Use after configuration changes or if diagnostics seem stale.`,
				Usage: heredoc.Doc(`
					Restarts a single named server or all servers if no name is given.
					Use when LSP servers are misbehaving or after changing project configuration.
				`),
				Examples: heredoc.Doc(`
					{}
					{"server": "gopls"}
				`),
			},
		},
		manager: manager,
	}
}

func (t *RestartTool) Name() string { return "LspRestart" }

// Sandboxable returns false because the tool communicates with in-process LSP servers
// via the manager pointer, which does not exist in the sandboxed child process.
func (t *RestartTool) Sandboxable() bool { return false }

// Parallel returns false because restarting LSP servers races with Diagnostics and Read's DidOpen.
func (t *RestartTool) Parallel() bool { return false }

func (t *RestartTool) Schema() tool.Schema {
	return tool.GenerateSchema[RestartInputs](t)
}

func (t *RestartTool) Available() bool {
	return t.manager != nil
}

func (t *RestartTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[RestartInputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	if params.Server != "" {
		if err := t.manager.RestartServer(ctx, params.Server); err != nil {
			return "", err
		}

		return fmt.Sprintf("LSP server %q restarted.", params.Server), nil
	}

	t.manager.RestartAll(ctx)

	return "All LSP servers restarted.", nil
}
