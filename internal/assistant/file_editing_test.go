package assistant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
