package sdk

import "reflect"

// Symbols is the Yaegi export map for the sdk package.
// Use via interp.Use(sdk.Symbols) to make SDK types available to plugins.
var Symbols = map[string]map[string]reflect.Value{
	"github.com/idelchi/aura/sdk/sdk": {
		// Constants
		"RoleUser":      reflect.ValueOf(RoleUser),
		"RoleAssistant": reflect.ValueOf(RoleAssistant),

		// Types (pointer-to-nil pattern for Yaegi type registration)
		"Result":                 reflect.ValueOf((*Result)(nil)),
		"ToolCall":               reflect.ValueOf((*ToolCall)(nil)),
		"Context":                reflect.ValueOf((*Context)(nil)),
		"BeforeChatContext":      reflect.ValueOf((*BeforeChatContext)(nil)),
		"AfterResponseContext":   reflect.ValueOf((*AfterResponseContext)(nil)),
		"BeforeToolContext":      reflect.ValueOf((*BeforeToolContext)(nil)),
		"BeforeToolResult":       reflect.ValueOf((*BeforeToolResult)(nil)),
		"AfterToolContext":       reflect.ValueOf((*AfterToolContext)(nil)),
		"OnErrorContext":         reflect.ValueOf((*OnErrorContext)(nil)),
		"AfterCompactionContext": reflect.ValueOf((*AfterCompactionContext)(nil)),
		"OnAgentSwitchContext":   reflect.ValueOf((*OnAgentSwitchContext)(nil)),
		"ResponseModification":   reflect.ValueOf((*ResponseModification)(nil)),
		"ErrorModification":      reflect.ValueOf((*ErrorModification)(nil)),
		"RequestModification":    reflect.ValueOf((*RequestModification)(nil)),
		"TokenState":             reflect.ValueOf((*TokenState)(nil)),
		"Stats":                  reflect.ValueOf((*Stats)(nil)),
		"ToolCount":              reflect.ValueOf((*ToolCount)(nil)),
		"ModelInfo":              reflect.ValueOf((*ModelInfo)(nil)),
		"FeatureState":           reflect.ValueOf((*FeatureState)(nil)),
		"SandboxFeatureState":    reflect.ValueOf((*SandboxFeatureState)(nil)),
		"Turn":                   reflect.ValueOf((*Turn)(nil)),

		// Tool types
		"ToolSchema":     reflect.ValueOf((*ToolSchema)(nil)),
		"ToolParameters": reflect.ValueOf((*ToolParameters)(nil)),
		"ToolProperty":   reflect.ValueOf((*ToolProperty)(nil)),
		"ToolPaths":      reflect.ValueOf((*ToolPaths)(nil)),
		"ToolConfig":     reflect.ValueOf((*ToolConfig)(nil)),

		// Command types
		"CommandSchema": reflect.ValueOf((*CommandSchema)(nil)),
		"CommandResult": reflect.ValueOf((*CommandResult)(nil)),

		// Compaction control
		"CompactionModification":  reflect.ValueOf((*CompactionModification)(nil)),
		"BeforeCompactionContext": reflect.ValueOf((*BeforeCompactionContext)(nil)),

		// Message transform types
		"MessageToolCall":  reflect.ValueOf((*MessageToolCall)(nil)),
		"Message":          reflect.ValueOf((*Message)(nil)),
		"TransformContext": reflect.ValueOf((*TransformContext)(nil)),
	},
}
