package todo_reminder

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

func BeforeChat(_ context.Context, ctx sdk.BeforeChatContext) (sdk.Result, error) {
	if !ctx.Auto {
		return sdk.Result{}, nil
	}

	interval := 20
	if v, ok := ctx.PluginConfig["interval"].(int); ok {
		interval = v
	}

	if interval <= 0 || ctx.Iteration == 0 || ctx.Iteration%interval != 0 {
		return sdk.Result{}, nil
	}

	if ctx.Todo.Pending == 0 && ctx.Todo.InProgress == 0 {
		return sdk.Result{}, nil
	}

	return sdk.Result{
		Message: heredoc.Doc(`
			I will consider the current todos and focus on completing the in_progress one.
			If certain tasks are already done, I MUST mark them accordingly NOW.

			If I have not recently reviewed the todo list, I will do so now.

			I will call 'TodoList' to see them.

			If I am already on track, I will keep going. If I have forgotten to mark previously completed tasks as completed, I will do so NOW.
		`),
		Prefix: "[SYSTEM FEEDBACK]: ",
	}, nil
}
