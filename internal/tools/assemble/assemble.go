// Package assemble provides the canonical tool assembly pipeline.
// All tool set construction — base tools + Task + Batch + ToolDefs — goes through
// this package, eliminating duplication across CLI inspection, session init, and
// per-turn config reload.
package assemble

import (
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/tools"
	"github.com/idelchi/aura/internal/tools/ask"
	"github.com/idelchi/aura/internal/tools/batch"
	"github.com/idelchi/aura/internal/tools/done"
	"github.com/idelchi/aura/internal/tools/task"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// Params holds dependencies for the canonical tool assembly pipeline.
type Params struct {
	Config      config.Config
	Paths       config.Paths
	Runtime     *config.Runtime // nil for CLI inspection
	TodoList    *todo.List
	Events      chan<- ui.Event
	PluginCache *plugins.Cache
	LSPManager  *lsp.Manager
	Estimate    func(string) int // from estimator.EstimateLocal

	// Existing tools to re-inject instead of creating new ones (reload path).
	// When nil, new instances are created from config.
	ExistingTask  *task.Tool
	ExistingBatch *batch.Tool

	// When true, adds Done(nil) and Ask(nil) for display/inspection.
	ForDisplay bool
}

// Result holds the assembled tool set and references to meta-tools.
type Result struct {
	Tools tool.Tools
	Task  *task.Tool // nil if no subagent agents exist
	Batch *batch.Tool
}

// Tools builds the complete tool set: base tools + Task + Batch + ToolDefs.
//
// Pipeline:
//  1. tools.All() — built-in + plugin + Skill + ToolDefs + availability filter
//  2. Task tool — re-inject existing or create from subagent agents
//  3. Batch tool — re-inject existing or create new
//  4. Display tools — Done(nil) + Ask(nil) when ForDisplay is set
//  5. ToolDefs.Apply — catches Task, Batch, and display tools
//  6. Runtime.AllTools assignment (when Runtime is non-nil)
func Tools(p Params) (Result, error) {
	all, err := tools.All(p.Config, p.Paths, p.Runtime, p.TodoList, p.Events, p.PluginCache, p.LSPManager, p.Estimate)
	if err != nil {
		return Result{}, err
	}

	// Task tool: re-inject existing or discover subagent agents and create.
	var taskTool *task.Tool

	if p.ExistingTask != nil {
		taskTool = p.ExistingTask
	} else {
		var agentInfos []task.AgentInfo

		for _, name := range p.Config.Agents.Filter(config.Agent.IsSubagent).Names() {
			ag := p.Config.Agents.Get(name)

			agentInfos = append(agentInfos, task.AgentInfo{
				Name:        ag.Metadata.Name,
				Description: ag.Metadata.Description,
			})
		}

		if len(agentInfos) > 0 {
			taskTool = task.New(agentInfos)
		}
	}

	if taskTool != nil {
		all.Add(taskTool)
	}

	// Batch tool: re-inject existing or create new.
	var batchTool *batch.Tool

	if p.ExistingBatch != nil {
		batchTool = p.ExistingBatch
	} else {
		batchTool = batch.New()
	}

	all.Add(batchTool)

	// Display-only tools (safe nil callbacks — for CLI inspection only).
	if p.ForDisplay {
		all.Add(done.New(nil))
		all.Add(ask.New(nil))
	}

	// Apply ToolDefs to newly added meta-tools (Task, Batch, Done, Ask).
	// tools.All() already applied ToolDefs to base tools — this is idempotent for those.
	p.Config.ToolDefs.Apply(&all)

	// Set runtime tool registry when available.
	if p.Runtime != nil {
		p.Runtime.AllTools = all
	}

	return Result{
		Tools: all,
		Task:  taskTool,
		Batch: batchTool,
	}, nil
}
