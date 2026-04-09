package done_reminder

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

func AfterResponse(_ context.Context, ctx sdk.AfterResponseContext) (sdk.Result, error) {
	if !ctx.DoneActive {
		return sdk.Result{}, nil
	}

	if ctx.Response.Empty {
		return sdk.Result{}, nil
	}

	if ctx.HasToolCalls {
		return sdk.Result{}, nil
	}

	return sdk.Result{
		Message: heredoc.Doc(`
			⚠️ To finish, you must verify that your TodoList is either non-existent, empty, or with all tasks set to complete.
			Then call the Done tool with your final message to the user.
		`),
		Role: sdk.RoleUser,
	}, nil
}
