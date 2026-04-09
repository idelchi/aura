package todo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// ProgressUpdateInput defines a single status update.
type ProgressUpdateInput struct {
	Index  int    `json:"index"  jsonschema:"required,description=1-based index of the todo item"                                       validate:"required,min=1"`
	Status string `json:"status" jsonschema:"required,enum=pending,enum=in_progress,enum=completed,description=New status for the item" validate:"required,oneof=pending in_progress completed"`
}

// ProgressInput defines the parameters for the TodoProgress tool.
type ProgressInput struct {
	Updates []ProgressUpdateInput `json:"updates" jsonschema:"required,description=List of status updates" validate:"required,min=1,dive"`
}

// Progress implements the TodoProgress tool.
type Progress struct {
	tool.Base

	list *todo.List
}

// NewProgress creates a new TodoProgress tool with the given list.
func NewProgress(list *todo.List) *Progress {
	return &Progress{
		Base: tool.Base{
			Text: tool.Text{
				Description: heredoc.Doc(`
					Update the status of todo items by index. Use when:
					- Completing a task (set to completed) - next pending task auto-starts
					- Starting a specific task (set to in_progress)
					- Task becomes blocked (set back to pending)

					When you complete a task, the next pending task automatically becomes in_progress.

					Only issue a 'completed' status update AFTER a todo task is done and verified/validated.

					You may ONLY mark a task as 'completed' if you are certain it is finished.
				`),
				Usage: heredoc.Doc(`
					Updates todo items by their 1-based index. Supports batch updates.
					When completing a task, the next pending task auto-starts.

					Patterns:
					- Complete a task: {"updates": [{"index": 1, "status": "completed"}]}
					- Start specific task: {"updates": [{"index": 3, "status": "in_progress"}]}
					- Batch update: {"updates": [{"index": 1, "status": "completed"}, {"index": 2, "status": "completed"}]}
				`),
				Examples: heredoc.Doc(`
					Complete task 1: {"updates": [{"index": 1, "status": "completed"}]}
					Start task 2: {"updates": [{"index": 2, "status": "in_progress"}]}
					Complete tasks 1 and 2: {"updates": [{"index": 1, "status": "completed"}, {"index": 2, "status": "completed"}]}
				`),
			},
		},
		list: list,
	}
}

// Name returns the tool's identifier.
func (t *Progress) Name() string {
	return "TodoProgress"
}

// Schema returns the tool definition.
func (t *Progress) Schema() tool.Schema {
	return tool.GenerateSchema[ProgressInput](t)
}

// Sandboxable returns false as todo tools manage in-memory state.
func (t *Progress) Sandboxable() bool {
	return false
}

// Parallel returns false because TodoProgress mutates the shared todo list without synchronization.
func (t *Progress) Parallel() bool {
	return false
}

// Execute updates todo item statuses.
func (t *Progress) Execute(_ context.Context, args map[string]any) (string, error) {
	params, err := tool.ValidateInput[ProgressInput](args, t.Schema())
	if err != nil {
		return "", err
	}

	if t.list.IsEmpty() {
		return "", errors.New("no todo list exists - use TodoCreate first")
	}

	items := t.list.Get()
	maxIndex := len(items)

	// Validate all indices first
	for _, update := range params.Updates {
		if update.Index < 1 || update.Index > maxIndex {
			return "", fmt.Errorf("invalid index %d: must be between 1 and %d", update.Index, maxIndex)
		}
	}

	// Snapshot before applying (for rollback on validation failure)
	snapshot := make([]todo.Todo, len(items))
	copy(snapshot, items)

	// Track completed and started tasks for verbose output
	var (
		completedTasks []int
		startedTasks   []int
	)

	for _, update := range params.Updates {
		switch update.Status {
		case "completed":
			completedTasks = append(completedTasks, update.Index)
		case "in_progress":
			startedTasks = append(startedTasks, update.Index)
		}
	}

	// Apply updates
	for _, update := range params.Updates {
		idx := update.Index - 1 // Convert to 0-based
		if err := t.list.MarkStatus(idx, todo.Status(update.Status)); err != nil {
			return "", fmt.Errorf("updating item %d: %w", update.Index, err)
		}
	}

	// Count by status after update
	pending, inProgress, completed := t.list.Counts()

	// Validate: only reject if >1 in_progress
	if inProgress > 1 {
		t.list.Replace(snapshot)

		var inProgressTasks []string

		for _, idx := range t.list.FindInProgress() {
			inProgressTasks = append(inProgressTasks, fmt.Sprintf("task %d", idx+1))
		}

		return "", fmt.Errorf(
			"REJECTED: Cannot have multiple tasks in_progress (found %d). Currently in_progress: %s. "+
				"Your update was NOT applied. Complete the current in_progress task before starting another",
			inProgress, strings.Join(inProgressTasks, ", "),
		)
	}

	// Auto-promote: if no in_progress but pending exist, promote first pending
	var (
		promotedIndex   int
		promotedContent string
	)

	if inProgress == 0 && pending > 0 {
		for i, item := range t.list.Get() {
			if item.Status == todo.Pending {
				t.list.MarkStatus(i, todo.InProgress)

				promotedIndex = i + 1
				promotedContent = item.Content
				inProgress = 1
				pending--

				break
			}
		}
	}

	// Build result message
	var result strings.Builder

	for _, idx := range completedTasks {
		fmt.Fprintf(&result, "Task %d completed.\n", idx)
	}

	if promotedIndex > 0 {
		fmt.Fprintf(&result, "Task %d now in_progress: %q\n", promotedIndex, promotedContent)
	} else if len(startedTasks) > 0 {
		currentItems := t.list.Get()

		for _, idx := range startedTasks {
			fmt.Fprintf(&result, "Task %d now in_progress: %q\n", idx, currentItems[idx-1].Content)
		}
	} else if pending == 0 && inProgress == 0 && completed > 0 {
		result.WriteString("All tasks completed!\n")
	}

	fmt.Fprintf(&result, "Status: %d pending, %d in_progress, %d completed.", pending, inProgress, completed)

	return result.String(), nil
}
