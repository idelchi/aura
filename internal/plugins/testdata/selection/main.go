package selection

import (
	"context"
	"github.com/idelchi/aura/sdk"
)

// BeforeChat supplies a recognizable hook receipt for selection tests.
func BeforeChat(_ context.Context, _ sdk.BeforeChatContext) (sdk.Result, error) {
	return sdk.Result{Message: "selection hook ran"}, nil
}

// Schema exposes a harmless tool for checking registry replacement.
func Schema() sdk.ToolSchema {
	return sdk.ToolSchema{Name: "SelectionProbe", Description: "Selection test", Parameters: sdk.ToolParameters{Type: "object"}}
}

// Execute supplies a recognizable tool receipt without side effects.
func Execute(_ context.Context, _ sdk.Context, _ map[string]any) (string, error) {
	return "selection tool ran", nil
}

// Command exposes a harmless command for checking selection and UI hints.
func Command() sdk.CommandSchema {
	return sdk.CommandSchema{Name: "selection-probe", Description: "Selection test", Hints: "[value]"}
}

// ExecuteCommand supplies a recognizable command receipt without side effects.
func ExecuteCommand(_ context.Context, _ string, _ sdk.Context) (sdk.CommandResult, error) {
	return sdk.CommandResult{Output: "selection command ran"}, nil
}
