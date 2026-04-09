package todo_not_finished

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

func AfterResponse(_ context.Context, ctx sdk.AfterResponseContext) (sdk.Result, error) {
	if ctx.HasToolCalls {
		return sdk.Result{}, nil
	}

	if !ctx.Auto {
		return sdk.Result{}, nil
	}

	if ctx.Todo.Pending == 0 && ctx.Todo.InProgress == 0 {
		return sdk.Result{}, nil
	}

	if ctx.Response.Empty {
		return sdk.Result{}, nil
	}

	return sdk.Result{
		Message: heredoc.Doc(`
			⚠️ I still have incomplete tasks in the ToDo list. I will call 'TodoList' to see them.
			I will continue with the next task by issuing the appropriate tool calls.
			If certain tasks are already done, I will make sure to mark them accordingly.
		`),
	}, nil
}
