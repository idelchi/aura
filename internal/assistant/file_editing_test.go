package assistant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/hooks"
	"github.com/idelchi/aura/internal/tools/batch"
	"github.com/idelchi/aura/internal/tools/write"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// Native file mutations must stay ordered through both assistant dispatch paths;
// a full write racing a later replacement can otherwise silently erase that edit.
func TestFileMutationsRunInOrder(t *testing.T) {
	for _, batched := range []bool{false, true} {
		t.Run(map[bool]string{false: "response", true: "batch"}[batched], func(t *testing.T) {
			a := selectionAssistant(t)
			a.toggles.auto = true
			a.agent.Tools.Add(write.New()) // explicitly offer the opt-in Write tool
			for _, name := range []string{"Write", "Edit", "Patch", "Batch"} {
				if a.IsToolParallel(name) {
					t.Fatalf("%s allows concurrent mutations", name)
				}
			}
			if !a.IsToolParallel("Read") {
				t.Fatal("file-mutation ordering unnecessarily disabled parallel reads")
			}
			path := filepath.Join(t.TempDir(), "file.txt")
			if err := os.WriteFile(path, []byte("initial\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			a.tracker.RecordRead(path)
			calls := []call.Call{
				{ID: "write", Name: "Write", Arguments: map[string]any{"path": path, "content": "left=old\nright=old\n"}},
				{ID: "edit", Name: "Edit", Arguments: map[string]any{"file_path": path, "old_string": "left=old", "new_string": "left=new", "replace_all": false}},
				{ID: "patch", Name: "Patch", Arguments: map[string]any{"patch": "*** Begin Patch\n*** Update File: " + path + "\n@@\n-right=old\n+right=new\n*** End Patch"}},
			}
			if batched {
				tool := batch.New()
				tool.Run, tool.IsParallel = a.ExecuteSubTool, a.IsToolParallel
				args := make([]batch.SubCall, len(calls))
				for i, c := range calls {
					args[i] = batch.SubCall{Name: c.Name, Arguments: c.Arguments}
				}
				output, err := tool.Execute(t.Context(), map[string]any{"calls": args})
				if err != nil || !strings.Contains(output, "3/3 successful") {
					t.Fatalf("batch: %s %v", output, err)
				}
			} else {
				a.executeTools(t.Context(), calls)
				if a.session.stats.Tools.Errors != 0 {
					t.Fatalf("tool errors: %d", a.session.stats.Tools.Errors)
				}
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != "left=new\nright=new\n" {
				t.Fatalf("file mutations lost or reordered: %q %v", got, err)
			}
		})
	}
}

func TestGroupedEditConfigurationAndDispatch(t *testing.T) {
	for _, batched := range []bool{false, true} {
		t.Run(map[bool]string{false: "response", true: "batch"}[batched], func(t *testing.T) {
			a := selectionAssistant(t)
			a.toggles.auto = true
			// Exercise changing the resolved feature after initial tool assembly.
			for _, multiple := range []bool{true, false, true} {
				a.globalFeatures.ToolExecution.EditMultiple = &multiple
				if err := a.RebuildState(); err != nil {
					t.Fatal(err)
				}
				edit, err := a.agent.Tools.Get("Edit")
				if err != nil {
					t.Fatal(err)
				}
				_, grouped := edit.Schema().Parameters.Properties.Get("edits")
				if grouped != multiple {
					t.Fatalf("Edit schema did not follow edit_multiple=%t", multiple)
				}
			}
			path := filepath.Join(t.TempDir(), "file.txt")
			if err := os.WriteFile(path, []byte("left=old\nright=old\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			a.tracker.RecordRead(path)
			receipt := filepath.Join(t.TempDir(), "hook.txt")
			postCalls := 0
			runner, err := hooks.New(config.Hooks{"inspect": {Event: "post", Matcher: "^Edit$", Command: fmt.Sprintf("cat %q >> %q", path, receipt)}})
			if err != nil {
				t.Fatal(err)
			}
			runner.OnStart = func(string) { postCalls++ }
			a.resolved.hooks = runner
			args := map[string]any{"file_path": path, "edits": []any{
				map[string]any{"old_string": "left=old", "new_string": "left=new"},
				map[string]any{"old_string": "right=old", "new_string": "right=new"},
			}}
			if batched {
				tool := batch.New()
				tool.Run, tool.IsParallel = a.ExecuteSubTool, a.IsToolParallel
				output, err := tool.Execute(t.Context(), map[string]any{"calls": []batch.SubCall{{Name: "Edit", Arguments: args}}})
				if err != nil || !strings.Contains(output, "1/1 successful") {
					t.Fatalf("batch: %q, %v", output, err)
				}
			} else {
				a.executeTools(t.Context(), []call.Call{{ID: "grouped", Name: "Edit", Arguments: args}})
				if a.session.stats.Tools.Errors != 0 {
					t.Fatalf("tool errors: %d", a.session.stats.Tools.Errors)
				}
			}
			got, err := os.ReadFile(receipt)
			if err != nil || string(got) != "left=new\nright=new\n" || postCalls != 1 {
				t.Fatalf("post hook must see the completed group once: %q, %v, calls=%d", got, err, postCalls)
			}
		})
	}
}
