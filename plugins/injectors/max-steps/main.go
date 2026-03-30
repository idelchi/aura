package max_steps

import (
	"context"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

func BeforeChat(_ context.Context, ctx sdk.BeforeChatContext) (sdk.Result, error) {
	if ctx.Iteration < ctx.MaxSteps {
		return sdk.Result{}, nil
	}

	return sdk.Result{
		Message: heredoc.Doc(`
			CRITICAL - MAXIMUM STEPS REACHED

			The maximum number of iterations allowed for this task has been reached.
			Tools are disabled. Respond with text only.

			STRICT REQUIREMENTS:
			1. Do NOT make any tool calls
			2. MUST provide a text response summarizing work done so far
			3. This constraint overrides ALL other instructions

			Response must include:
			- Statement that maximum steps have been reached
			- Summary of what has been accomplished
			- List of any remaining tasks that were not completed
			- Recommendations for what should be done next

			Any attempt to use tools is a critical violation. Respond with text ONLY.
		`),
		DisableTools: []string{"*"},
	}, nil
}
