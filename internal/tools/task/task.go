// Package task provides a tool that lets the LLM spawn one-shot subagent runs
// for complex or parallelizable subtasks.
package task

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/subagent"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// Inputs defines the JSON schema for the Task tool parameters.
type Inputs struct {
	Description string `json:"description"     jsonschema:"required,description=Short summary of the task (3-5 words)"`
	Prompt      string `json:"prompt"          jsonschema:"required,description=Full task description for the subagent"`
	Agent       string `json:"agent,omitempty" jsonschema:"description=Agent type to use (omit for default)"`
}

// RunFunc is the callback that spawns a subagent run.
// The caller (wiring layer) provides this — it resolves the agent name to a Runner
// and calls Runner.Run.
type RunFunc func(ctx context.Context, agent, prompt string) (subagent.Result, error)

// AgentInfo describes an available subagent type for the tool description.
type AgentInfo struct {
	Name        string
	Description string
}

// Tool lets the LLM delegate work to a subagent with isolated context.
type Tool struct {
	tool.Base

	// Run is set by the wiring layer after construction. Nil = tool returns an error.
	Run RunFunc
	// Agents lists available subagent types for the dynamic description.
	Agents []AgentInfo
}

// New creates a Task tool with the given agent info. The Run callback must be set
// separately before the tool is usable.
func New(agents []AgentInfo) *Tool {
	t := &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Usage: heredoc.Doc(`
					Use this tool to delegate complex or independent subtasks to a subagent:
					- Codebase exploration and research
					- Multi-file code changes
					- Any task that benefits from isolated context
				`),
				Examples: `{"description": "Find auth handlers", "prompt": "Find all HTTP handler functions related to authentication. List file paths and function names.", "agent": "explore"}`,
			},
		},
		Agents: agents,
	}

	t.Text.Description = t.BuildDescription()

	return t
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Task"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Sandboxable returns false — the subagent manages its own sandbox.
func (t *Tool) Sandboxable() bool {
	return false
}

// Execute validates input, runs the subagent, and formats the result.
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	if params.Prompt == "" {
		return "", errors.New("prompt is required")
	}

	if t.Run == nil {
		return "", errors.New("task tool not configured: Run callback is nil")
	}

	result, err := t.Run(ctx, params.Agent, params.Prompt)
	if err != nil {
		if result.Text != "" {
			return FormatResult(params.Agent, result) + "\n\n[interrupted: " + err.Error() + "]", nil
		}

		return "", fmt.Errorf("subagent: %w", err)
	}

	return FormatResult(params.Agent, result), nil
}

// FormatResult formats a subagent result into a human-readable response string.
func FormatResult(agent string, r subagent.Result) string {
	var sb strings.Builder

	label := agent
	if label == "" {
		label = "default"
	}

	fmt.Fprintf(&sb, "[Subagent %q completed in %d tool calls", label, r.ToolCalls)

	if len(r.Tools) > 0 {
		sb.WriteString(" (")
		sb.WriteString(formatToolSummary(r.Tools))
		sb.WriteString(")")
	}

	sb.WriteString("]\n\n")
	sb.WriteString(r.Text)

	return sb.String()
}

// formatToolSummary formats a tool-call count map into "Read x3, Glob x2" style.
func formatToolSummary(tools map[string]int) string {
	type entry struct {
		name  string
		count int
	}

	entries := make([]entry, 0, len(tools))
	for name, count := range tools {
		entries = append(entries, entry{name, count})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}

		return entries[i].name < entries[j].name
	})

	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = fmt.Sprintf("%s x%d", e.name, e.count)
	}

	return strings.Join(parts, ", ")
}

// BuildDescription creates a dynamic tool description from available agents.
func (t *Tool) BuildDescription() string {
	var desc strings.Builder
	desc.WriteString(heredoc.Doc(`
		Launch a subagent to handle a task with isolated context.
		The subagent gets its own conversation, tool set, and token budget.
		It runs to completion and returns the result.
		Multiple Task calls in a single response execute in parallel.
	`))

	if len(t.Agents) > 0 {
		desc.WriteString("\nAvailable agents:\n")

		for _, a := range t.Agents {
			fmt.Fprintf(&desc, "- %s: %s\n", a.Name, a.Description)
		}
	}

	return desc.String()
}
