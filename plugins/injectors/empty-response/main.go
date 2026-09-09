package empty_response

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

// AfterResponse requests a usable answer; optionally close tool access to keep
// an empty response from starting another retrieval loop.
func AfterResponse(_ context.Context, ctx sdk.AfterResponseContext) (sdk.Result, error) {
	if !ctx.Response.Empty {
		return sdk.Result{}, nil
	}
	var disabled []string
	switch values := ctx.PluginConfig["disable_tools"].(type) {
	case []string:
		disabled = values
	case []any:
		for _, value := range values {
			if pattern, ok := value.(string); ok {
				disabled = append(disabled, pattern)
			}
		}
	}
	if len(disabled) > 0 {
		return sdk.Result{
			Message:      "My response was empty. The configured tools are now disabled for this turn. I must finish using existing evidence, or explicitly state that the task is incomplete. Other available tools may still be used for required actions. I must not claim an unconfirmed action succeeded.",
			DisableTools: disabled,
		}, nil
	}

	return sdk.Result{
		Message: heredoc.Doc(`
			⚠️ My response was empty. I will continue with the task or explain what I need.
		`),
	}, nil
}
