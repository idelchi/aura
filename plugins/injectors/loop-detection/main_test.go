package loop_detection

import (
	"testing"

	"github.com/idelchi/aura/sdk"
)

// TestLoopDisabling preserves changed-argument retries and ordinary failures.
func TestLoopDisabling(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		enabled, different, failed bool
		want                       bool
	}{
		{name: "advisory"},
		{name: "repeated success", enabled: true, want: true},
		{name: "smaller tail", enabled: true, different: true},
		{name: "failed execution", enabled: true, failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := sdk.AfterToolContext{}
			ctx.PluginConfig = map[string]any{"window": 2, "disable_tool": tc.enabled}
			ctx.ToolHistory = []sdk.ToolCall{{Name: "logs", ArgsJSON: `{"tail":50}`}, {Name: "logs", ArgsJSON: `{"tail":50}`}}
			if tc.different {
				ctx.ToolHistory[1].ArgsJSON = `{"tail":25}`
			}
			if tc.failed {
				ctx.ToolHistory[1].Error = "unavailable"
			}
			got, err := AfterToolExecution(t.Context(), ctx)
			if err != nil || (len(got.DisableTools) > 0) != tc.want {
				t.Fatalf("result=%+v err=%v", got, err)
			}
		})
	}
}
