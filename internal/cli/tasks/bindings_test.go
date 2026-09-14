package tasks

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
)

// TestTaskBindingsRuntime uses the same task definition for two independent runs.
// Runtime values must resolve after env without replacing its reusable templates.
func TestTaskBindingsRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bindings.yaml")
	if err := os.WriteFile(path, []byte(`base:
  env:
    STAMP: "$[[ .RUN ]]"
  features:
    tools:
      bindings:
        Bash:
          command: "printf $[[ .STAMP | shellQuote ]]"
  commands: ["!true"]
bindings:
  inherit: [base]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	var definitions task.Tasks
	if err := definitions.Load(files.Files{file.New(path)}, nil); err != nil {
		t.Fatal(err)
	}
	definition := definitions["bindings"]
	for _, stamp := range []string{"first", "second"} {
		home := filepath.Join(t.TempDir(), ".aura")
		if err := os.CopyFS(home, os.DirFS("../../../tests/fixtures/base/.aura")); err != nil {
			t.Fatal(err)
		}
		definition.Vars = map[string]string{"RUN": stamp}
		flags := core.Flags{Home: home, Agent: "Test", Dry: "noop", WithoutPlugins: true, Writer: io.Discard, IsSet: core.NotSet}
		err := core.RunSession(t.Context(), flags, core.Selection{}, core.HeadlessUI,
			func(ctx context.Context, _ context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error {
				go u.Run(ctx) //nolint:errcheck
				if err := runTask(io.Discard, ctx, asst, u, definition, false, "", 0); err != nil {
					return err
				}
				output, err := asst.ExecuteSubTool(ctx, "Bash", map[string]any{"command": "printf wrong"})
				if err != nil || !strings.Contains(output, stamp) || strings.Contains(output, "wrong") {
					return fmt.Errorf("run %s: %q (%v)", stamp, output, err)
				}
				return nil
			})
		if err != nil {
			t.Fatal(err)
		}
	}
}
