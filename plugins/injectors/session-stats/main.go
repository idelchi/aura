package session_stats

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/sdk"
)

// BeforeChat injects a stats summary every N turns (default 5).
func BeforeChat(_ context.Context, hctx sdk.BeforeChatContext) (sdk.Result, error) {
	interval := 5
	if v, ok := hctx.PluginConfig["interval"].(int); ok {
		interval = v
	}

	if interval <= 0 || hctx.Stats.Turns == 0 || hctx.Stats.Turns%interval != 0 {
		return sdk.Result{}, nil
	}

	topTools := 3
	if v, ok := hctx.PluginConfig["top_tools"].(int); ok {
		topTools = v
	}

	return sdk.Result{
		Notice: fmt.Sprintf(
			"--- session stats (turn %d, %s elapsed) ---\n"+
				"Tools: %d calls (%d failed)\n"+
				"Context: estimate=%d lastAPI=%d max=%d (%.0f%%)\n"+
				"Top tools: %s",
			hctx.Stats.Turns,
			hctx.Stats.Duration,
			hctx.Stats.Tools.Calls,
			hctx.Stats.Tools.Errors,
			hctx.Tokens.Estimate,
			hctx.Tokens.LastAPI,
			hctx.Tokens.Max,
			hctx.Tokens.Percent,
			formatTopTools(hctx.Stats.TopTools, topTools),
		),
	}, nil
}

// formatTopTools formats the top N tools as "name(count)" entries.
func formatTopTools(tools []sdk.ToolCount, n int) string {
	if len(tools) == 0 {
		return "none"
	}

	if n > len(tools) {
		n = len(tools)
	}

	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = fmt.Sprintf("%s(%d)", tools[i].Name, tools[i].Count)
	}

	return strings.Join(parts, ", ")
}
