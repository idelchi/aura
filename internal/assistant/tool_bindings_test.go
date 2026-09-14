package assistant

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// TestToolBindingsLifecycle exercises the real tool registry and execution paths.
func TestToolBindingsLifecycle(t *testing.T) {
	a := selectionAssistant(t)
	features := config.Features{ToolExecution: config.ToolExecution{Bindings: tool.Bindings{"Bash": {"command": "printf fixed"}}}}
	for _, rebuild := range []string{"merge", "reload"} {
		if rebuild == "merge" {
			if err := a.MergeFeatures(features); err != nil {
				t.Fatal(err)
			}
		} else if err := a.Reload(t.Context()); err != nil {
			t.Fatal(err)
		}
		probe, err := a.agent.Tools.Get("Bash")
		if err != nil {
			t.Fatal(err)
		}
		if _, visible := probe.Schema().Parameters.Properties["command"]; visible {
			t.Fatal("fixed command visible after", rebuild)
		}
		for name, execute := range map[string]func() (string, error){
			"batch": func() (string, error) {
				return a.ExecuteSubTool(t.Context(), "Bash", map[string]any{"command": "printf wrong"})
			},
			"delegate": func() (string, error) {
				return a.subagentExecuteOverride(nil)(t.Context(), probe, map[string]any{"command": "printf wrong"})
			},
		} {
			output, err := execute()
			if err != nil || !strings.Contains(output, "fixed") || strings.Contains(output, "wrong") {
				t.Fatalf("%s/%s: %q (%v)", rebuild, name, output, err)
			}
		}
		a.executeTools(t.Context(), []call.Call{{ID: rebuild, Name: "Bash", Arguments: map[string]any{"command": "printf wrong"}}})
		found := false
		for _, msg := range a.builder.History() {
			if msg.ToolCallID == rebuild && strings.Contains(msg.Content, "fixed") {
				found = true
			}
		}
		if !found {
			t.Fatal("main execution did not return fixed command output")
		}
	}
	features.ToolExecution.Deferred = []string{"Bash"}
	if err := a.MergeFeatures(features); err != nil {
		t.Fatal(err)
	}
	deferred, err := a.tools.deferred.Get("Bash")
	if err != nil {
		t.Fatal(err)
	}
	if _, visible := deferred.Schema().Parameters.Properties["command"]; visible {
		t.Fatal("deferred schema leaked fixed input")
	}
	if a.rt.LoadedTools == nil {
		a.rt.LoadedTools = make(map[string]bool)
	}
	a.rt.LoadedTools["Bash"] = true
	if err := a.RebuildState(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ExecuteSubTool(t.Context(), "Bash", nil); err != nil {
		t.Fatal(err)
	}
	if err := a.MergeFeatures(config.Features{ToolExecution: config.ToolExecution{Bindings: tool.Bindings{}}}); err != nil {
		t.Fatal(err)
	}
	probe, err := a.agent.Tools.Get("Bash")
	if err != nil {
		t.Fatal(err)
	}
	if _, visible := probe.Schema().Parameters.Properties["command"]; !visible {
		t.Fatal("clearing bindings did not restore schema")
	}
}
