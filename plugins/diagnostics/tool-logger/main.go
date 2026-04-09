package tool_logger

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/sdk"
)

// AfterToolExecution fires after every tool call.
// Injects a loud summary of what happened — tool name, result preview, error if any.
func AfterToolExecution(_ context.Context, hctx sdk.AfterToolContext) (sdk.Result, error) {
	previewError := 200
	if v, ok := hctx.PluginConfig["preview_length_error"].(int); ok {
		previewError = v
	}

	previewOK := 100
	if v, ok := hctx.PluginConfig["preview_length_ok"].(int); ok {
		previewOK = v
	}

	if hctx.Tool.Error != "" {
		return sdk.Result{
			Message: fmt.Sprintf(
				"=== TOOL FAILED [%d/%d errors] ===\nTool: %s\nError: %s\nResult preview: %s",
				hctx.Stats.Tools.Errors, hctx.Stats.Tools.Calls+hctx.Stats.Tools.Errors,
				hctx.Tool.Name,
				hctx.Tool.Error,
				Truncate(hctx.Tool.Result, previewError),
			),
			Prefix: "[tool-logger] ",
			Eject:  true,
		}, nil
	}

	return sdk.Result{
		Message: fmt.Sprintf(
			"=== TOOL OK [%d calls] === %s → %s",
			hctx.Stats.Tools.Calls,
			hctx.Tool.Name,
			Truncate(hctx.Tool.Result, previewOK),
		),
		Prefix: "[tool-logger] ",
		Eject:  true,
	}, nil
}

// OnError fires when the LLM provider returns an error.
func OnError(_ context.Context, hctx sdk.OnErrorContext) (sdk.Result, error) {
	return sdk.Result{
		Message: fmt.Sprintf(
			"!!! LLM ERROR on iteration %d !!!\nError: %s\nContext: %.0f%% used (%d tokens)\nTotal tool calls this session: %d (%d errors)",
			hctx.Iteration,
			hctx.Error,
			hctx.Tokens.Percent,
			hctx.Tokens.Estimate,
			hctx.Stats.Tools.Calls,
			hctx.Stats.Tools.Errors,
		),
		Role:   sdk.RoleUser,
		Prefix: "[tool-logger] ",
		Eject:  false, // keep error context visible across turns
	}, nil
}
