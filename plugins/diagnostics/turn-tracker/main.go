package turn_tracker

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/sdk"
)

var lastResponse int

// BeforeChat fires before every LLM call.
// Injects a visible status line every N turns (default 3) showing turn count and context usage.
func BeforeChat(_ context.Context, hctx sdk.BeforeChatContext) (sdk.Result, error) {
	interval := 3
	if v, ok := hctx.PluginConfig["interval"].(int); ok {
		interval = v
	}

	if interval <= 0 || hctx.Stats.Turns == 0 || hctx.Stats.Turns%interval != 0 {
		return sdk.Result{}, nil
	}

	return sdk.Result{
		Message: fmt.Sprintf(
			"=== TURN %d | context: %d tokens (%.0f%%) | agent: %s | model: %s | messages: %d ===",
			hctx.Stats.Turns,
			hctx.Tokens.Estimate,
			hctx.Tokens.Percent,
			hctx.Agent,
			hctx.ModelInfo.Name,
			hctx.MessageCount,
		),
		Role:   sdk.RoleUser,
		Prefix: "[turn-tracker] ",
		Eject:  true,
	}, nil
}

// AfterResponse fires after every LLM response.
// Tracks response length and warns if responses exceed max_response_length (default 5000).
func AfterResponse(_ context.Context, hctx sdk.AfterResponseContext) (sdk.Result, error) {
	lastResponse = len(hctx.Content)

	maxLen := 5000
	if v, ok := hctx.PluginConfig["max_response_length"].(int); ok {
		maxLen = v
	}

	if lastResponse > maxLen {
		return sdk.Result{
			Message: fmt.Sprintf(
				"Last response was %d characters — that's very long. Consider asking for shorter answers.",
				lastResponse,
			),
			Prefix: "[turn-tracker] ",
		}, nil
	}

	return sdk.Result{}, nil
}
