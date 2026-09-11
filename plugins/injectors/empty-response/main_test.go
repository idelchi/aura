package empty_response

import (
	"testing"

	"github.com/idelchi/aura/sdk"
)

func TestEmptyRecoveryBound(t *testing.T) {
	ctx := sdk.AfterResponseContext{}
	ctx.Response.Empty = true
	ctx.PluginConfig = map[string]any{"retries": 1}
	ctx.Response.EmptyCount = 1
	first, _ := AfterResponse(t.Context(), ctx)
	if first.Stop != "" || first.Message == "" {
		t.Fatal("first recovery was not offered")
	}
	ctx.Response.EmptyCount = 2
	last, _ := AfterResponse(t.Context(), ctx)
	if last.Stop == "" {
		t.Fatal("empty recovery was not bounded")
	}
}

// TestEmptyDisablesSelectedTools leaves notification tools available and only
// changes behavior for a truly empty response.
func TestEmptyDisablesSelectedTools(t *testing.T) {
	for _, empty := range []bool{false, true} {
		ctx := sdk.AfterResponseContext{}
		ctx.Response.Empty = empty
		ctx.PluginConfig = map[string]any{"disable_tools": []any{"logs"}}
		got, err := AfterResponse(t.Context(), ctx)
		if err != nil || (len(got.DisableTools) == 1) != empty {
			t.Fatalf("result=%+v err=%v", got, err)
		}
		if empty && got.DisableTools[0] != "logs" {
			t.Fatal("disabled unrelated tools")
		}
	}
	ctx := sdk.AfterResponseContext{}
	ctx.Response.Empty = true
	got, err := AfterResponse(t.Context(), ctx)
	if err != nil || got.Message == "" || len(got.DisableTools) != 0 {
		t.Fatal("default nudge changed")
	}
}
