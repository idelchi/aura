package assistant

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// sessionAssistant is a configured assistant that persists sessions in dir, as a CLI run does.
func sessionAssistant(t *testing.T, dir string) *Assistant {
	t.Helper()
	a := selectionAssistant(t)
	a.session.manager = session.NewManager(session.NewStore(dir))
	return a
}

// savedSessions loads every session persisted in dir.
func savedSessions(t *testing.T, dir string) []*session.Session {
	t.Helper()
	store := session.NewStore(dir)
	listing, err := store.List()
	if err != nil || len(listing.Warnings) > 0 {
		t.Fatalf("listing sessions: %v %v", err, listing.Warnings)
	}
	sessions := make([]*session.Session, 0, len(listing.Summaries))
	for _, summary := range listing.Summaries {
		saved, err := store.Load(summary.ID)
		if err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, saved)
	}
	return sessions
}

// hasMessage reports whether msgs holds a message with the given role and content.
func hasMessage(msgs message.Messages, role roles.Role, content string) bool {
	for _, msg := range msgs {
		if msg.Role == role && msg.Content == content {
			return true
		}
	}
	return false
}

// hasToolResult reports whether msgs holds the result of the given call, paired with that call.
func hasToolResult(msgs message.Messages, id, content string) bool {
	called := false
	for _, msg := range msgs {
		for _, c := range msg.Calls {
			called = called || c.ID == id
		}
		if called && msg.Role == roles.Tool && msg.ToolCallID == id && msg.Content == content {
			return true
		}
	}
	return false
}

// Non-interactive input is persisted by the shutdown auto-save once it is admitted, without /save.
// A provider failure after a completed tool keeps that history resumable under the same ID.
func TestAdmittedInputAutoSaves(t *testing.T) {
	dir := t.TempDir()
	a := sessionAssistant(t, dir)
	crash := errors.New("engine exited with status 6")
	chats := 0
	a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(_ request.Request) (message.Message, usage.Usage, error) {
		chats++
		if chats == 1 {
			return message.Message{Calls: []call.Call{{ID: "probe", Name: "SelectionProbe", Arguments: map[string]any{}}}}, usage.Usage{}, nil
		}
		return message.Message{}, usage.Usage{}, crash
	}}
	if err := a.ProcessInput(t.Context(), "Run the teaching prompt"); !errors.Is(err, crash) || chats != 2 {
		t.Fatalf("chats=%d error=%v; want the provider failure after one tool", chats, err)
	}
	// RunSession auto-saves after its work returns, including on error.
	if err := a.AutoSave(); err != nil {
		t.Fatal(err)
	}
	saved := savedSessions(t, dir)
	if len(saved) != 1 {
		t.Fatalf("saved %d sessions after a failed fresh run, want 1", len(saved))
	}
	id := saved[0].ID
	if !hasMessage(saved[0].Messages, roles.User, "Run the teaching prompt") ||
		!hasToolResult(saved[0].Messages, "probe", "selection tool ran") {
		t.Fatalf("failed run lost its user/tool history: %+v", saved[0].Messages)
	}

	// Resume in a fresh process and complete the work.
	b := sessionAssistant(t, dir)
	resumed, err := b.session.manager.Resume(id)
	if err != nil {
		t.Fatal(err)
	}
	if warnings := b.ResumeSession(t.Context(), resumed); len(warnings) > 0 {
		t.Fatalf("resume warnings: %v", warnings)
	}
	b.agent.Provider = compactionRequestProbe{Provider: b.agent.Provider, chat: func(req request.Request) (message.Message, usage.Usage, error) {
		if !hasToolResult(req.Messages, "probe", "selection tool ran") {
			t.Errorf("resumed request lacks the earlier tool result: %+v", req.Messages)
		}
		return message.Message{Content: "Completed."}, usage.Usage{}, nil
	}}
	if err := b.ProcessInput(t.Context(), "Continue"); err != nil {
		t.Fatal(err)
	}
	if err := b.AutoSave(); err != nil {
		t.Fatal(err)
	}
	saved = savedSessions(t, dir)
	if len(saved) != 1 || saved[0].ID != id {
		t.Fatalf("resume did not update session %s: %d sessions", id, len(saved))
	}
	history := saved[0].Messages
	if !hasMessage(history, roles.User, "Run the teaching prompt") || !hasToolResult(history, "probe", "selection tool ran") ||
		!hasMessage(history, roles.User, "Continue") || !hasMessage(history, roles.Assistant, "Completed.") {
		t.Fatalf("resumed session lost or did not extend its history: %+v", history)
	}
}

// Input that never enters the conversation leaves no session behind.
func TestUnadmittedInputDoesNotAutoSave(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		rejected    bool
	}{
		{name: "blank", input: " \n "},
		{name: "command", input: "/selection-probe"},
		{name: "oversized", input: strings.Repeat("evidence ", 60000), rejected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a := sessionAssistant(t, dir)
			chats := 0
			a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(req request.Request) (message.Message, usage.Usage, error) {
				if len(req.Tools) > 0 {
					chats++
				}
				return message.Message{Content: "Unexpected."}, usage.Usage{}, nil
			}}
			err := a.ProcessInput(t.Context(), tc.input)
			if (err != nil) != tc.rejected || chats != 0 {
				t.Fatalf("chats=%d error=%v; want rejected=%t without a chat", chats, err, tc.rejected)
			}
			if err := a.AutoSave(); err != nil {
				t.Fatal(err)
			}
			if saved := savedSessions(t, dir); len(saved) != 0 {
				t.Fatalf("unadmitted input created %d sessions", len(saved))
			}
		})
	}
}

// The interactive loop follows the same rule: only admitted input makes a session worth saving.
func TestInteractiveInputAutoSave(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        int
	}{
		{name: "blank", input: " \n ", want: 0},
		{name: "admitted", input: "Say hello", want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a := sessionAssistant(t, dir)
			events := make(chan ui.Event, 1000)
			a.events = events
			a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(_ request.Request) (message.Message, usage.Usage, error) {
				return message.Message{Content: "Hello."}, usage.Usage{}, nil
			}}
			inputs := make(chan ui.UserInput)
			loop := make(chan error, 1)
			go func() { loop <- a.Loop(t.Context(), inputs, nil, nil) }()
			inputs <- ui.UserInput{Text: tc.input}
			for done := false; !done; {
				select {
				case event := <-events:
					if finished, ok := event.(ui.AssistantDone); ok {
						if finished.Error != nil {
							t.Fatal(finished.Error)
						}
						done = true
					}
				case <-time.After(10 * time.Second):
					t.Fatal("turn did not finish")
				}
			}
			close(inputs)
			if err := <-loop; err != nil {
				t.Fatal(err)
			}
			if err := a.AutoSave(); err != nil {
				t.Fatal(err)
			}
			if saved := savedSessions(t, dir); len(saved) != tc.want {
				t.Fatalf("saved %d sessions, want %d", len(saved), tc.want)
			}
		})
	}
}
