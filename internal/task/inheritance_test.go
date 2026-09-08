package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
)

// TestForEachInheritance replaces a supplied block and preserves an omitted one.
func TestForEachInheritance(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tasks.yaml")
	content := heredoc.Doc(`
		parent:
		  commands: ["/new"]
		  foreach:
		    shell: echo all-containers
		    continue_on_error: true
		    retries: 2
		child:
		  inherit: [parent]
		  foreach:
		    shell: echo selected-containers
		grandchild:
		  inherit: [child]
		file-child:
		  inherit: [parent]
		  foreach:
		    file: selected.txt
	`)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var tasks Tasks
	if err := tasks.Load(files.Files{file.New(path)}, nil); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"child", "grandchild"} {
		got := tasks[name].ForEach
		if got == nil || *got != (ForEach{Shell: "echo selected-containers"}) {
			t.Errorf("%s foreach = %+v, want only the child's shell", name, got)
		}
	}
	if got := tasks["parent"].ForEach; got == nil || *got != (ForEach{
		Shell: "echo all-containers", ContinueOnError: true, Retries: 2,
	}) {
		t.Errorf("parent foreach was changed: %+v", got)
	}
	if got := tasks["file-child"].ForEach; got == nil || *got != (ForEach{File: "selected.txt"}) {
		t.Errorf("file-child foreach = %+v, want file source without inherited shell", got)
	}
}
