package simple_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/internal/ui/simple"
)

func TestSimpleShutdownNeverApproves(t *testing.T) {
	s := simple.New(ui.Status{Agent: "test"}, filepath.Join(t.TempDir(), "history"))

	ask := make(chan string, 1)
	confirm := make(chan ui.ConfirmAction, 1)

	s.Events() <- ui.AskRequired{Question: "Which?", Options: []ui.AskOption{{Label: "first"}}, Response: ask}
	s.Events() <- ui.ToolConfirmRequired{ToolName: "Bash", Response: confirm}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("simple UI did not stop")
	}

	select {
	case got := <-ask:
		if got != "" {
			t.Errorf("question answered %q at shutdown, want dismissed", got)
		}
	default:
		t.Error("question left blocked at shutdown")
	}

	select {
	case got := <-confirm:
		if got != ui.ConfirmDeny {
			t.Errorf("confirmation answered %v at shutdown, want denied", got)
		}
	default:
		t.Error("confirmation left blocked at shutdown")
	}
}
