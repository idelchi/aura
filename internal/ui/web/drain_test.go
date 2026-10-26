package web

import (
	"testing"

	"github.com/idelchi/aura/internal/ui"
)

func TestWebDrainNeverApproves(t *testing.T) {
	w := New("127.0.0.1:0")

	ask := make(chan string, 1)
	confirm := make(chan ui.ConfirmAction, 1)

	w.Events() <- ui.AskRequired{Question: "Which?", Options: []ui.AskOption{{Label: "first"}}, Response: ask}
	w.Events() <- ui.ToolConfirmRequired{ToolName: "Bash", Response: confirm}

	w.drain()

	if got := <-ask; got != "" {
		t.Errorf("question answered %q at shutdown, want dismissed", got)
	}

	if got := <-confirm; got != ui.ConfirmDeny {
		t.Errorf("confirmation answered %v at shutdown, want denied", got)
	}
}
