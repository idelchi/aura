package todo

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// CreateItemInput defines a single todo item for creation.
type CreateItemInput struct {
	Content string `json:"content" jsonschema:"required,description=Brief description of the task" validate:"required"`
}

// CreateInput defines the parameters for the TodoCreate tool.
type CreateInput struct {
	Summary string            `json:"summary,omitempty" jsonschema:"description=Executive summary describing the overall goal"`
	Items   []CreateItemInput `json:"items,omitempty"   jsonschema:"description=List of tasks to create"                       validate:"omitempty,dive"`
}

// Create implements the TodoCreate tool.
type Create struct {
	tool.Base

	list *todo.List
}

// NewCreate creates a new TodoCreate tool with the given list.
func NewCreate(list *todo.List) *Create {
	return &Create{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Create or update a todo list for multi-step tasks.

					Items start as pending. First item is auto-set to in_progress.
				`),
				Usage: heredoc.Doc(`
					Creates or updates a todo list.

					Parameters:
					- summary: Executive summary of the overall goal
					- items: List of task descriptions

					Behavior:
					- {summary, items}: Set both summary and tasks
					- {summary}: Update summary only, keep existing tasks
					- {items}: Replace tasks only, keep existing summary

					First item is automatically marked as in_progress.
				`),
				Examples: heredoc.Doc(`
					{"summary": "Implement user auth", "items": [{"content": "Add login endpoint"}, {"content": "Add JWT validation"}]}
					{"summary": "Refactoring the config system"}
					{"items": [{"content": "Parse config"}, {"content": "Validate schema"}]}
				`),
			},
		},
		list: list,
	}
}

// Name returns the tool's identifier.
func (t *Create) Name() string {
	return "TodoCreate"
}

// Schema returns the tool definition.
func (t *Create) Schema() tool.Schema {
	return tool.GenerateSchema[CreateInput](t)
}

// Sandboxable returns false as todo tools manage in-memory state.
func (t *Create) Sandboxable() bool {
	return false
}

// Parallel returns false because TodoCreate mutates the shared todo list without synchronization.
func (t *Create) Parallel() bool {
	return false
}

// Execute creates or updates the todo list.
func (t *Create) Execute(_ context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[CreateInput](args, t.Schema())
	if err != nil {
		return "", err
	}

	if params.Summary == "" && len(params.Items) == 0 {
		return "", errors.New("must provide summary or items (or both)")
	}

	var result string

	if params.Summary != "" {
		t.list.SetSummary(params.Summary)
	}

	if len(params.Items) > 0 {
		todos := make([]todo.Todo, len(params.Items))
		for i, item := range params.Items {
			status := todo.Pending

			if i == 0 {
				status = todo.InProgress
			}

			todos[i] = todo.Todo{
				Content: item.Content,
				Status:  status,
			}
		}

		t.list.Replace(todos)

		result = fmt.Sprintf("Created %d tasks. First task is now in_progress.", len(todos))
	}

	if params.Summary != "" {
		if result != "" {
			result += "\n\n"
		}

		result += "Summary: " + params.Summary
	}

	return result, nil
}
