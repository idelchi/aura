package core

import "testing"

// TestSelectionOverrides verifies task defaults without falsifying flag provenance.
func TestSelectionOverrides(t *testing.T) {
	t.Parallel()
	for _, explicit := range []bool{false, true} {
		flags := Flags{Agent: "cli-agent", Mode: "", Model: "cli-model", Provider: "cli-provider"}
		flags.IsSet = func(name string) bool {
			return name == "model" || name == "provider" || explicit && (name == "agent" || name == "mode")
		}
		got, err := (Selection{Agent: "task-agent", Mode: "task-mode"}).overrides(flags)
		if err != nil {
			t.Fatal(err)
		}
		wantAgent, wantMode := "task-agent", "task-mode"
		if explicit {
			wantAgent, wantMode = "cli-agent", ""
		}
		if got.Agent == nil || *got.Agent != wantAgent || got.Mode == nil || *got.Mode != wantMode {
			t.Fatalf("explicit=%v: unexpected selection: %+v", explicit, got)
		}
		if *got.Model != flags.Model || *got.Provider != flags.Provider {
			t.Fatal("lost explicit model/provider selection")
		}
		if flags.IsSet("agent") != explicit || flags.IsSet("mode") != explicit || flags.Agent != "cli-agent" {
			t.Fatal("mutated scheduler flags or flag provenance")
		}
	}
}
