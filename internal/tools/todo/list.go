package todo

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// ListInput defines the parameters for the TodoList tool.
type ListInput struct{}

// List implements the TodoList tool.
type List struct {
	tool.Base

	list *todo.List
}

// NewList creates a new TodoList tool with the given list.
func NewList(list *todo.List) *List {
	return &List{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Read the current todo list. Use this tool proactively and frequently:
					- At the start of work to see pending tasks
					- Before starting a new task
					- When uncertain what to do next
					- After completing tasks to see remaining work
					- Every few tool calls to stay on track
				`),
				Usage: heredoc.Doc(`
					Returns the current todo list with status for each item.
					No parameters required - just call the tool.

					Use this frequently to maintain awareness of task progress.
				`),
				Examples: `{}`,
			},
		},
		list: list,
	}
}

// Name returns the tool's identifier.
func (t *List) Name() string {
	return "TodoList"
}

// Schema returns the tool definition.
func (t *List) Schema() tool.Schema {
	return tool.GenerateSchema[ListInput](t)
}

// Sandboxable returns false as todo tools manage in-memory state.
func (t *List) Sandboxable() bool {
	return false
}

// Execute returns the current todo list.
func (t *List) Execute(_ context.Context, _ map[string]any) (string, error) {
	if t.list.IsEmpty() {
		return "No todos.", nil
	}

	pending, inProgress, completed := t.list.Counts()

	var result string

	if t.list.Len() > 0 {
		result = fmt.Sprintf(
			"Todo List (%d pending, %d in_progress, %d completed):\n\n",
			pending,
			inProgress,
			completed,
		)
	}

	result += t.list.String()

	return result, nil
}
