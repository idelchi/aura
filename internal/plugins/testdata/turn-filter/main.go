package turn_filter

import (
	"context"

	"github.com/idelchi/aura/sdk"
)

// AfterResponse withdraws the probe before dispatch when the response requests it.
func AfterResponse(_ context.Context, sc sdk.AfterResponseContext) (sdk.Result, error) {
	if sc.Content == "restrict probe" {
		return sdk.Result{DisableTools: []string{"SelectionProbe"}}, nil
	}
	return sdk.Result{}, nil
}

// AfterToolExecution withdraws the probe after its first successful execution.
func AfterToolExecution(_ context.Context, sc sdk.AfterToolContext) (sdk.Result, error) {
	if sc.Tool.Name == "SelectionProbe" && sc.Tool.Error == "" {
		return sdk.Result{DisableTools: []string{"SelectionProbe"}}, nil
	}
	return sdk.Result{}, nil
}
