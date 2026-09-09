package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/cli/core"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/internal/ui"
)

// TestSessionTaskDeadline exercises the session boundary used by scheduled tasks,
// including an expired final item and cancellation-independent post cleanup.
func TestSessionTaskDeadline(t *testing.T) {
	for _, items := range []string{"one", "one\\ntwo"} {
		t.Run(items, func(t *testing.T) {
			dir := t.TempDir()
			home := filepath.Join(dir, ".aura")
			if err := os.CopyFS(home, os.DirFS("../../../tests/fixtures/base/.aura")); err != nil {
				t.Fatal(err)
			}
			flags := core.Flags{Home: home, Agent: "Test", Dry: "noop", WithoutPlugins: true, Writer: io.Discard, IsSet: core.NotSet}
			output := filepath.Join(dir, "items")
			post := filepath.Join(dir, "post")
			definition := task.Task{
				Name: "deadline", Schedule: "once: startup", Timeout: 5 * time.Second,
				ForEach:  &task.ForEach{Shell: "printf '" + items + "\\n'", ContinueOnError: true, Retries: 1},
				Commands: []string{`!printf '%s\n' {{ .Item | shellQuote }} >> "${OUTPUT}"; sleep 10`},
				Post:     []string{`printf '%s' {{ .Result | toJson | shellQuote }} > "${POST}"`},
				Env:      map[string]string{"OUTPUT": output, "POST": post},
			}
			// Use a parent deadline, just as the scheduler does. Starting the short
			// work deadline inside the callback would not test the broken boundary.
			results := make(chan error, 1)
			scheduler, err := task.NewScheduler(task.Tasks{definition.Name: definition}, 1, func(ctx context.Context, scheduled task.Task) error {
				err := core.RunSession(ctx, flags, core.Selection{}, core.HeadlessUI,
					func(sessionCtx context.Context, _ context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error {
						deadline, ok := sessionCtx.Deadline()
						want, _ := ctx.Deadline()
						if !ok || !deadline.Equal(want) {
							return errors.New("session lost its caller's deadline")
						}
						go u.Run(sessionCtx) //nolint:errcheck
						return runTask(io.Discard, sessionCtx, asst, u, scheduled, false, "", 0)
					})
				results <- err
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = scheduler.Shutdown() })
			scheduler.Start()
			select {
			case err = <-results:
			case <-time.After(20 * time.Second):
				t.Fatal("scheduled task did not stop after its deadline")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("task lost deadline cause: %v", err)
			}
			got, err := os.ReadFile(output)
			if err != nil || strings.TrimSpace(string(got)) != "one" {
				t.Fatalf("expired task continued items or retries: %q (%v)", got, err)
			}
			got, err = os.ReadFile(post)
			if err != nil {
				t.Fatalf("post cleanup did not run: %q (%v)", got, err)
			}
			var result task.Result
			if err := json.Unmarshal(got, &result); err != nil {
				t.Fatal(err)
			}
			wantTotal := strings.Count(items, "\\n") + 1
			if result.Status != "timed_out" || result.Total != wantTotal || result.Failed != 1 || result.Completed != 0 || result.Unprocessed != wantTotal-1 || result.Items[0].Attempts != 1 {
				t.Fatalf("incorrect execution receipt: %+v", result)
			}
		})
	}
}
