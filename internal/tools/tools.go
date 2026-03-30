package tools

import (
	"fmt"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/tools/bash"
	edittool "github.com/idelchi/aura/internal/tools/edit"
	"github.com/idelchi/aura/internal/tools/glob"
	"github.com/idelchi/aura/internal/tools/ls"
	memread "github.com/idelchi/aura/internal/tools/memory/read"
	memwrite "github.com/idelchi/aura/internal/tools/memory/write"
	"github.com/idelchi/aura/internal/tools/mkdir"
	"github.com/idelchi/aura/internal/tools/patch"
	querytool "github.com/idelchi/aura/internal/tools/query"
	"github.com/idelchi/aura/internal/tools/read"
	"github.com/idelchi/aura/internal/tools/ripgrep"
	skilltool "github.com/idelchi/aura/internal/tools/skill"
	speaktool "github.com/idelchi/aura/internal/tools/speak"
	todotools "github.com/idelchi/aura/internal/tools/todo"
	tokenstool "github.com/idelchi/aura/internal/tools/tokens"
	transcribetool "github.com/idelchi/aura/internal/tools/transcribe"
	visiontool "github.com/idelchi/aura/internal/tools/vision"
	webfetchtool "github.com/idelchi/aura/internal/tools/webfetch"
	websearchtool "github.com/idelchi/aura/internal/tools/websearch"
	writetool "github.com/idelchi/aura/internal/tools/write"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// ToolResult represents structured output for tool execution.
type ToolResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
	Setup    bool   `json:"setup,omitempty"`
}

// SetupError signals that a sandboxed subprocess failed during setup
// (config, plugins, estimator), not during tool execution.
// The LLM should not see these — they are routed to the user instead.
type SetupError struct{ Err error }

func (e *SetupError) Error() string { return e.Err.Error() }
func (e *SetupError) Unwrap() error { return e.Err }

// All returns all built-in tools with runtime-dependent configuration applied.
// Meta-tools (Ask, Done, LoadTools) are injected later by rebuildState().
// Batch is always registered; Task is conditional (only if subagent agents exist).
// None of those meta-tools are returned here — only the base set.
// pluginCache is optional (nil when plugins are disabled).
// lspManager is optional (nil when LSP is disabled or not needed).
func All(
	cfg config.Config,
	paths config.Paths,
	rt *config.Runtime,
	todoList *todo.List,
	events chan<- ui.Event,
	pluginCache *plugins.Cache,
	lspManager *lsp.Manager,
	estimate func(string) int,
) (tool.Tools, error) {
	qt := querytool.New(cfg, paths, rt)

	qt.Events = events

	tools := tool.Tools{
		bash.New(cfg.Features.ToolExecution.Bash.Truncation, cfg.Features.ToolExecution.Bash.Rewrite),
		edittool.New(),
		glob.New(),
		ls.New(),
		memread.New(paths.Home, paths.Global),
		memwrite.New(paths.Home, paths.Global),
		mkdir.New(),
		patch.New(),
		read.New(cfg.Features.ToolExecution.ReadSmallFileTokens, lspManager, estimate),
		ripgrep.New(),
		todotools.NewCreate(todoList),
		todotools.NewList(todoList),
		todotools.NewProgress(todoList),
		visiontool.New(cfg, paths, rt),
		transcribetool.New(cfg, paths, rt),
		speaktool.New(cfg, paths, rt),
		webfetchtool.New(cfg.Features.ToolExecution.WebFetchMaxBodySize),
		websearchtool.New(),
		writetool.New(),
		tokenstool.New(cfg, paths, rt),
		lsp.NewDiagnosticsTool(lspManager),
		lsp.NewRestartTool(lspManager),
		qt,
	}

	if len(cfg.Skills) > 0 {
		tools = append(tools, skilltool.New(cfg.Skills))
	}

	pluginCount := len(pluginCache.Tools())

	if err := registerPluginTools(&tools, pluginCache); err != nil {
		return nil, err
	}

	cfg.ToolDefs.Apply(&tools)

	available := filterAvailable(tools)

	debug.Log(
		"[tools] registered %d built-in, %d plugin, %d available after filtering",
		len(tools)-pluginCount,
		pluginCount,
		len(available),
	)

	return available, nil
}

// filterAvailable removes tools whose preconditions are not met.
func filterAvailable(ts tool.Tools) tool.Tools {
	var available tool.Tools

	for _, t := range ts {
		if t.Available() {
			available = append(available, t)
		}
	}

	return available
}

// registerPluginTools adds plugin-exported tools to the tool set.
// Handles conflict detection with override support.
func registerPluginTools(ts *tool.Tools, pluginCache *plugins.Cache) error {
	for _, pt := range pluginCache.Tools() {
		if ts.Has(pt.Name()) {
			if !pt.Overrides() {
				return fmt.Errorf(
					"plugin tool %q conflicts with existing tool — set override: true in plugin.yaml to replace it",
					pt.Name(),
				)
			}

			ts.Remove(pt.Name())
		}

		ts.Add(pt)
	}

	return nil
}
