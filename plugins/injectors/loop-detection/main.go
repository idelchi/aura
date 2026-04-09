package loop_detection

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

func AfterToolExecution(_ context.Context, ctx sdk.AfterToolContext) (sdk.Result, error) {
	window := 2
	if v, ok := ctx.PluginConfig["window"].(int); ok {
		window = v
	}

	n := len(ctx.ToolHistory)
	if n < window {
		return sdk.Result{}, nil
	}

	// Check if the last `window` entries all have the same name and args.
	ref := ctx.ToolHistory[n-1]
	for i := n - 2; i >= n-window; i-- {
		entry := ctx.ToolHistory[i]
		if entry.Name != ref.Name || entry.ArgsJSON != ref.ArgsJSON {
			return sdk.Result{}, nil
		}
	}

	return sdk.Result{
		Message: heredoc.Docf(`
			⚠️ LOOP DETECTED: I called %q with identical arguments %d times in a row.
			This usually means I'm not making progress.
			I will STOP repeating this call and try a different approach.
		`, ref.Name, window),
		Prefix: "[SYSTEM FEEDBACK]: ",
	}, nil
}
