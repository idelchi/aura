package tasks

import (
	"testing"

	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/task"
)

// TestTaskSessionFlags checks task selection without losing explicit model/provider flags.
func TestTaskSessionFlags(t *testing.T) {
	t.Parallel()
	for _, explicit := range []bool{false, true} {
		flags := core.Flags{Agent: "default", Model: "requested-model", Provider: "requested-provider"}
		flags.IsSet = func(name string) bool { return explicit && (name == "agent" || name == "mode") }
		flags.Tasks.Run.IsSet = func(string) bool { return false }
		definition := task.Task{Agent: "task-agent", Mode: "task-mode"}
		applyTaskOverrides(&definition, flags)
		got := taskSessionFlags(flags, definition)
		want := "task-agent"
		if explicit {
			want = "default"
		}
		if got.Agent != want || got.Model != flags.Model || got.Provider != flags.Provider {
			t.Errorf("explicit=%v: unexpected selection: agent=%s model=%s provider=%s", explicit, got.Agent, got.Model, got.Provider)
		}
		if flags.Agent != "default" {
			t.Error("mutated shared scheduler flags")
		}
		if !explicit && (got.Mode != "task-mode" || !got.IsSet("mode") || flags.IsSet("mode")) {
			t.Error("task mode was not scoped to the task's session")
		}
	}
}
