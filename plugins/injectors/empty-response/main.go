package empty_response

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

func AfterResponse(_ context.Context, ctx sdk.AfterResponseContext) (sdk.Result, error) {
	if !ctx.Response.Empty {
		return sdk.Result{}, nil
	}

	return sdk.Result{
		Message: heredoc.Doc(`
			⚠️ My response was empty. I will continue with the task or explain what I need.
		`),
	}, nil
}
