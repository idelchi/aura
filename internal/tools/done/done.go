// Package done provides a tool for the LLM to explicitly signal task completion.
package done

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/pkg/llm/tool"
)

// Inputs defines the parameters for the Done tool.
type Inputs struct {
	Summary string `json:"summary" jsonschema:"required,description=Final summary of what was accomplished"`
}

// Tool signals task completion. When called, it sets a flag that exits the assistant loop.
type Tool struct {
	tool.Base

	// OnDone is called when the LLM invokes this tool.
	// The assistant sets this callback to signal loop exit.
	OnDone func(summary string)
}

// New creates a Done tool with the given callback.
func New(onDone func(string)) *Tool {
	return &Tool{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Call this tool when you have completed all tasks and want to exit tool calling enforcement.
					Provide a final summary of what was accomplished.
				`),
				Usage: heredoc.Doc(`
					Use this tool when you have finished all your tasks and are ready to provide a final response.
					Before calling Done, verify that your TodoList is either non-existent, empty, or with all tasks set to complete.
				`),
				Examples: `{"summary": "Created the new auth package with JWT support and wired it into the HTTP middleware."}`,
			},
		},
		OnDone: onDone,
	}
}

// Name returns the tool's identifier.
func (t *Tool) Name() string {
	return "Done"
}

// Schema returns the provider-agnostic tool definition.
func (t *Tool) Schema() tool.Schema {
	return tool.GenerateSchema[Inputs](t)
}

// Execute signals task completion via the OnDone callback.
func (t *Tool) Execute(_ context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[Inputs](args, t.Schema())
	if err != nil {
		return "", err
	}

	if t.OnDone != nil {
		t.OnDone(params.Summary)
	}

	return "Done", nil
}

// Sandboxable returns false as this tool has no filesystem operations.
func (t *Tool) Sandboxable() bool {
	return false
}

// Parallel returns false because Done sets the doneSignaled flag and must be the last tool executed.
func (t *Tool) Parallel() bool {
	return false
}
