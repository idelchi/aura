// Package batch provides a meta-tool that executes multiple independent tool calls concurrently.
package batch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/truncate"
)

// SubCall represents a single tool invocation within a batch.
type SubCall struct {
	Name      string         `json:"name"      jsonschema:"required,description=Name of the tool to execute"`
	Arguments map[string]any `json:"arguments" jsonschema:"required,description=Arguments for the tool"`
}

// Inputs defines the JSON schema for the Batch tool parameters.
type Inputs struct {
	Calls []SubCall `json:"calls" jsonschema:"required,description=Array of independent tool calls to execute concurrently" validate:"required,min=1,max=25,dive"`
}

// RunFunc executes a single sub-tool call through the assistant's minimal pipeline.
type RunFunc func(ctx context.Context, toolName string, args map[string]any) (string, error)

// disallowed lists tools that cannot run inside a batch.
var disallowed = map[string]bool{
	"Batch":     true, // no recursion
	"Ask":       true, // blocks for user input
	"Done":      true, // signals loop exit
	"Task":      true, // subagent with own loop
	"LoadTools": true, // triggers state rebuild
}

// Tool lets the LLM dispatch multiple independent tool calls in parallel.
type Tool struct {
	tool.Base

	// Run is set by the wiring layer after construction. Nil = tool returns an error.
	Run RunFunc

	// IsParallel reports whether a tool is safe for concurrent execution.
	// When nil, all tools are dispatched concurrently (test compatibility).
	// When set, tools returning false are executed sequentially after parallel ones complete.
	IsParallel func(name string) bool
}

// New creates a Batch tool. The Run callback must be set separately before the tool is usable.
func New() *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Executes multiple independent tool calls concurrently for faster results.

					Use this when you need to perform several independent operations — reading
					multiple files, searching in different paths, running multiple commands —
					and none depends on another's output.

					Good use cases:
					- Read several files at once
					- Search + glob + read combinations
					- Multiple independent bash commands
					- Multi-part edits on different files

					Do NOT use when:
					- One operation depends on another's result
					- Operations must happen in a specific order
					- You're modifying the same file in multiple calls

					1-25 tool calls per batch. All calls start in parallel; ordering is NOT
					guaranteed. Partial failures do not stop other calls.
				`),
				Examples: heredoc.Doc(`
					{"calls": [{"name": "Read", "arguments": {"file_path": "/tmp/a.go"}}, {"name": "Read", "arguments": {"file_path": "/tmp/b.go"}}]}
					{"calls": [{"name": "Glob", "arguments": {"pattern": "**/*.go"}}, {"name": "Bash", "arguments": {"command": "go mod tidy"}}]}
				`),
			},
		},
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Batch"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Sandboxable returns false — sub-calls handle their own sandboxing individually.
func (t *Tool) Sandboxable() bool {
	return false
}

// subResult holds the outcome of a single sub-call.
type subResult struct {
	Name   string
	Output string
	Error  string
}

// isNonParallel reports whether a tool must run sequentially.
// Returns false when IsParallel is nil (all tools concurrent — test compatibility).
func (t *Tool) isNonParallel(name string) bool {
	return t.IsParallel != nil && !t.IsParallel(name)
}

// Execute validates input, dispatches sub-calls with a two-pass pattern:
// parallel-safe tools run concurrently, then non-parallel tools run sequentially.
func (t *Tool) Execute(ctx context.Context, params map[string]any) (string, error) {
	input, err := tool.ValidateInput[Inputs](params, t.Schema())
	if err != nil {
		return "", err
	}

	if t.Run == nil {
		return "", errors.New("batch tool not configured: Run callback is nil")
	}

	results := make([]subResult, len(input.Calls))

	// Pre-fill disallowed tool results.
	for i, call := range input.Calls {
		if disallowed[call.Name] {
			results[i] = subResult{Name: call.Name, Error: fmt.Sprintf("tool %q cannot be batched", call.Name)}
		}
	}

	// Pass 1: dispatch parallel-safe tools concurrently.
	var wg sync.WaitGroup

	for i, call := range input.Calls {
		if disallowed[call.Name] || t.isNonParallel(call.Name) {
			continue
		}

		wg.Go(func() {
			output, err := t.Run(ctx, call.Name, call.Arguments)
			if err != nil {
				results[i] = subResult{Name: call.Name, Error: err.Error()}
			} else {
				results[i] = subResult{Name: call.Name, Output: output}
			}
		})
	}

	wg.Wait()

	// Pass 2: execute non-parallel tools sequentially.
	for i, call := range input.Calls {
		if disallowed[call.Name] || !t.isNonParallel(call.Name) {
			continue
		}

		output, err := t.Run(ctx, call.Name, call.Arguments)
		if err != nil {
			results[i] = subResult{Name: call.Name, Error: err.Error()}
		} else {
			results[i] = subResult{Name: call.Name, Output: output}
		}
	}

	return formatResults(input.Calls, results), nil
}

// formatResults renders batch results as markdown with per-call sections.
func formatResults(calls []SubCall, results []subResult) string {
	var b strings.Builder

	success := 0
	failed := 0

	for _, r := range results {
		if r.Error != "" {
			failed++
		} else {
			success++
		}
	}

	total := success + failed
	if failed > 0 {
		fmt.Fprintf(&b, "## Batch Results (%d/%d successful, %d failed)\n\n", success, total, failed)
	} else {
		fmt.Fprintf(&b, "## Batch Results (%d/%d successful)\n\n", success, total)
	}

	for i, r := range results {
		key := keyArg(calls[i].Arguments)
		fmt.Fprintf(&b, "### [%d] %s", i+1, r.Name)

		if key != "" {
			fmt.Fprintf(&b, " (%s)", key)
		}

		b.WriteString("\n")

		if r.Error != "" {
			fmt.Fprintf(&b, "Error: %s\n\n", r.Error)
		} else {
			fmt.Fprintf(&b, "%s\n\n", r.Output)
		}
	}

	return b.String()
}

// keyArg extracts the most descriptive argument value for display in result headers.
func keyArg(args map[string]any) string {
	for _, key := range []string{"path", "pattern", "command", "query", "url"} {
		if v, ok := args[key].(string); ok && v != "" {
			return truncate.Truncate(v, 63)
		}
	}

	return ""
}
