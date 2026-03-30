package repeated_patch

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/idelchi/aura/sdk"
)

func AfterToolExecution(_ context.Context, ctx sdk.AfterToolContext) (sdk.Result, error) {
	threshold := 5
	if v, ok := ctx.PluginConfig["threshold"].(int); ok {
		threshold = v
	}

	for file, count := range ctx.PatchCounts {
		if count >= threshold {
			return sdk.Result{
				Message: fmt.Sprintf("File %q patched %d times. ", file, count) + heredoc.Doc(`
					⚠️ I have attempted to patch the same file multiple times in succession. I MUST now read the file fully and rewrite it instead of patching it again.
				`),
				Prefix: "[SYSTEM FEEDBACK]: ",
			}, nil
		}
	}

	return sdk.Result{}, nil
}
