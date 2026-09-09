package loop_detection

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

// AfterToolExecution warns about repeated calls and can disable a looping tool
// after identical successful executions. Failed calls remain retryable.
func AfterToolExecution(_ context.Context, ctx sdk.AfterToolContext) (sdk.Result, error) {
	window := 2
	if v, ok := ctx.PluginConfig["window"].(int); ok {
		window = v
	}
	if window < 2 {
		window = 2
	}

	n := len(ctx.ToolHistory)
	if n < window {
		return sdk.Result{}, nil
	}

	// Check if the last `window` entries all have the same name and args.
	ref := ctx.ToolHistory[n-1]
	successful := ref.Error == ""
	for i := n - 2; i >= n-window; i-- {
		entry := ctx.ToolHistory[i]
		if entry.Name != ref.Name || entry.ArgsJSON != ref.ArgsJSON {
			return sdk.Result{}, nil
		}
		successful = successful && entry.Error == ""
	}
	if disable, _ := ctx.PluginConfig["disable_tool"].(bool); disable && successful {
		return sdk.Result{
			Message:      heredoc.Docf("The tool %q repeated the same executed call %d times and is disabled for this turn. Assess the evidence already returned. Execution with omitted output is not readable evidence. If evidence is insufficient, state that limitation; do not claim missing work was completed.", ref.Name, window),
			Prefix:       "[SYSTEM FEEDBACK]: ",
			DisableTools: []string{ref.Name},
		}, nil
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
