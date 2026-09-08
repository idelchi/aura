package tasks

import (
	"maps"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/config"
)

// runtimeData combines the current assistant context with task-local variables.
// Shared fields are reserved; task vars and environment remain accessible by key
// and through .Vars. Recompute before execution so /model and /agent changes are visible.
func runtimeData(asst *assistant.Assistant, vars map[string]string) map[string]any {
	data := asst.TemplateData()
	data.Vars = config.ToAnyMap(vars)
	result := config.ToAnyMap(vars)
	if result == nil {
		result = make(map[string]any)
	}
	maps.Copy(result, data.Context())
	return result
}
