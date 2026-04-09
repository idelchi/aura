package failure_circuit_breaker

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

func AfterToolExecution(_ context.Context, ctx sdk.AfterToolContext) (sdk.Result, error) {
	threshold := 3
	if v, ok := ctx.PluginConfig["max_failures"].(int); ok {
		threshold = v
	}

	consecutive := 0
	for i := len(ctx.ToolHistory) - 1; i >= 0; i-- {
		if ctx.ToolHistory[i].Error == "" {
			break
		}
		consecutive++
	}

	if consecutive < threshold {
		return sdk.Result{}, nil
	}

	return sdk.Result{
		Message: heredoc.Docf(`
			You have failed %d consecutive tool calls with different errors.
			This suggests your current approach is fundamentally wrong.

			STOP and reconsider:
			- What are you actually trying to achieve?
			- Why are these tools failing?
			- Is there a completely different approach?

			Do NOT continue retrying variations of the same strategy.
		`, consecutive),
		Prefix: "[SYSTEM FEEDBACK]: ",
	}, nil
}
