package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
)

// TestRuntimeTemplatesSurviveLoading keeps nested runtime expressions out of both load-time passes.
func TestRuntimeTemplatesSurviveLoading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	data := heredoc.Doc(`
		base:
		  env: {MODEL: "$[[ .Model.Name ]]"}
		  pre: ["echo $[[ .Provider.URL | shellQuote ]]"]
		  commands: ["$[[ .Model.Name ]]"]
		  post: ["echo $[[ .Provider.Name ]]"]
		  foreach: {shell: "echo $[[ .Provider.Name ]]"}
		  finally: ["$[[ .Model.Name ]]"]
		  on_max_steps: ["echo $[[ .Model.Name ]]"]
		child:
		  inherit: [base]
	`)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	// Exercise map iteration orders and inherited slices; conversion must not
	// expose one task's runtime expressions to another task's load-time pass.
	for range 20 {
		var tasks Tasks
		if err := tasks.Load(files.Files{file.New(path)}, nil); err != nil {
			t.Fatal(err)
		}
		for name, task := range tasks {
			for _, value := range []string{task.Pre[0], task.Commands[0], task.Post[0], task.ForEach.Shell, task.Finally[0], task.OnMaxSteps[0], task.Env["MODEL"]} {
				if !strings.Contains(value, "{{ .") || strings.Contains(value, "$[[") {
					t.Fatalf("%s: runtime expression lost: %s", name, value)
				}
			}
		}
	}
}

// TestPostTimeoutInheritance preserves defaults, inheritance and explicit overrides.
func TestPostTimeoutInheritance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	data := heredoc.Doc(`
		base:
		  commands: ["/new"]
		  post: ["echo cleanup"]
		parent:
		  inherit: [base]
		  post_timeout: 2s
		child:
		  inherit: [parent]
		grandchild:
		  inherit: [child]
		  post_timeout: 4s
	`)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	var tasks Tasks
	if err := tasks.Load(files.Files{file.New(path)}, nil); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]time.Duration{"base": 30 * time.Second, "parent": 2 * time.Second, "child": 2 * time.Second, "grandchild": 4 * time.Second} {
		if tasks[name].PostDeadline() != want || len(tasks[name].Post) != 1 {
			t.Fatalf("%s: %+v", name, tasks[name])
		}
	}
	for _, value := range []string{"0s", "-1s", "invalid"} {
		if err := os.WriteFile(path, []byte("bad:\n  commands: [/new]\n  post_timeout: "+value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := tasks.Load(files.Files{file.New(path)}, nil); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
