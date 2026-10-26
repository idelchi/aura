package core

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/roles"
)

// TestRunSessionAutoSave drives the non-interactive session boundary, as `aura run` does, with the
// noop provider. Admitted input is saved on exit without /save, even when the work fails afterwards.
func TestRunSessionAutoSave(t *testing.T) {
	crash := errors.New("engine exited with status 6")
	for _, tc := range []struct {
		name   string
		inputs []string
		fail   error
		want   int
	}{
		{name: "admitted input", inputs: []string{"Say hello"}, want: 1},
		{name: "failure after admitted input", inputs: []string{"Say hello"}, fail: crash, want: 1},
		{name: "blank input", inputs: []string{"   "}, want: 0},
		{name: "command only", inputs: []string{"/stats"}, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), ".aura")
			if err := os.CopyFS(home, os.DirFS("../../../tests/fixtures/base/.aura")); err != nil {
				t.Fatal(err)
			}
			flags := Flags{Home: home, Agent: "Test", Dry: "noop", WithoutPlugins: true, Writer: io.Discard, IsSet: NotSet}
			err := RunSession(t.Context(), flags, Selection{}, HeadlessUI,
				func(ctx context.Context, _ context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error {
					go u.Run(ctx) //nolint:errcheck
					for _, input := range tc.inputs {
						if err := asst.ProcessInput(ctx, input); err != nil {
							return err
						}
					}
					return tc.fail
				})
			if !errors.Is(err, tc.fail) {
				t.Fatalf("session error = %v, want %v", err, tc.fail)
			}
			store := session.NewStore(filepath.Join(home, "sessions"))
			listing, err := store.List()
			if err != nil || len(listing.Summaries) != tc.want {
				t.Fatalf("saved %d sessions (%v), want %d", len(listing.Summaries), err, tc.want)
			}
			if tc.want == 0 {
				return
			}
			saved, err := store.Load(listing.Summaries[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			answered := false
			for _, msg := range saved.Messages {
				answered = answered || msg.Role == roles.Assistant && msg.Content == "[noop]"
			}
			if saved.FirstUserContent() != "Say hello" || !answered {
				t.Fatalf("saved session lacks the turn: %+v", saved.Messages)
			}
		})
	}
}
