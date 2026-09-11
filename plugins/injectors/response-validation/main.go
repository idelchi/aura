// Package response_validation enforces the agent's declared response format.
package response_validation

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/sdk"
)

// AfterResponse allows bounded correction of invalid final responses.
// Tool-using responses are not final and are left untouched.
func AfterResponse(_ context.Context, ctx sdk.AfterResponseContext) (sdk.Result, error) {
	if len(ctx.Calls) > 0 || ctx.Response.ValidationError == "" {
		return sdk.Result{}, nil
	}
	retries := 1
	if value, ok := ctx.PluginConfig["retries"].(int); ok {
		retries = value
		if retries < 0 {
			retries = 0
		}
	}
	// Iteration is a per-turn bound: tool work does not grant additional retries.
	if ctx.Iteration > retries {
		return sdk.Result{Stop: "response format validation failed: " + ctx.Response.ValidationError}, nil
	}
	return sdk.Result{
		Message:      fmt.Sprintf("The final response does not satisfy the declared response format: %s. Return only the required result. Do not repeat completed actions.", ctx.Response.ValidationError),
		DisableTools: []string{"*"},
	}, nil
}
