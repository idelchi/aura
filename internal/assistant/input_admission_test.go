package assistant

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/idelchi/aura/internal/slash/commands"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
)

func TestCompactWithInternalMessages(t *testing.T) {
	a := selectionAssistant(t)
	a.cfg.Features.Compaction.Prompt = "Compaction"
	a.cfg.Features.Compaction.Chunks = 1
	a.resolved.compactModel = &model.Model{Name: "test"}
	a.builder.AddUserMessage(t.Context(), "Remember the requirements", 5)
	a.builder.AddBookmark("old turn")
	a.builder.AddAssistantMessage(message.Message{Content: "Recent answer"})
	a.builder.AddDisplayMessage(t.Context(), roles.Assistant, "UI only")
	a.builder.AddBookmark("latest turn")
	calls := 0
	a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(req request.Request) (message.Message, usage.Usage, error) {
		calls++
		for _, msg := range req.Messages {
			if msg.IsInternal() || msg.Content == "Recent answer" {
				t.Fatalf("unexpected compaction input: %+v", msg)
			}
		}
		return message.Message{Content: "Requirements retained."}, usage.Usage{}, nil
	}}
	if err := a.CompactWith(t.Context(), true, 10); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("compaction calls = %d, want 1", calls)
	}
	found := false
	for _, msg := range a.builder.History() {
		found = found || msg.Content == "Recent answer"
	}
	if !found {
		t.Fatal("recent answer was not preserved")
	}
}

func TestSplitHistoryToolBatchWithInternalMessages(t *testing.T) {
	history := message.Messages{
		{Role: roles.System, Content: "system"},
		{Role: roles.User, Content: "old request"},
		{Role: roles.Assistant, Calls: []call.Call{{ID: "one", Name: "Read"}}},
		{Role: roles.System, Type: message.Bookmark, Content: "UI bookmark"},
		{Role: roles.Tool, ToolCallID: "one", Content: "result"},
		{Role: roles.Assistant, Type: message.DisplayOnly, Content: "UI output"},
	}
	compact, preserved := splitHistory(history, 1)
	if len(compact) != 1 || compact[0].Content != "old request" || len(preserved) != 2 ||
		len(preserved[0].Calls) != 1 || preserved[0].Calls[0].ID != preserved[1].ToolCallID {
		t.Fatalf("tool batch split: compact=%+v preserved=%+v", compact, preserved)
	}
}

// Input admission exercises the real per-turn reload, compaction and /until path.
// The provider controls summaries only; no model judgement is needed to test progress.
func TestInputAdmissionCompaction(t *testing.T) {
	for _, tc := range []struct {
		name    string
		summary string
		failure error
		until   bool
		accept  bool
		batch   bool
	}{
		{name: "space recovered", summary: "Facts retained.", accept: true},
		{name: "summary still oversized", summary: strings.Repeat("oversized ", 40000)},
		{name: "until stops on rejected input", summary: strings.Repeat("oversized ", 40000), until: true},
		{name: "until stops with oversized recent tool batch", summary: "Earlier facts.", until: true, batch: true},
		{name: "compaction failure", failure: errors.New("summary unavailable")},
		{name: "cancellation preserved", failure: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := selectionAssistant(t)
			path := filepath.Join(a.configOpts.WriteHome, "config/features/compaction.yaml")
			if err := os.WriteFile(path, []byte(heredoc.Doc(`
				compaction:
				  prompt: Compaction
				  keep_last_messages: 1
				  chunks: 1
				  threshold: 99
				  prune:
				    mode: off
			`)), 0o600); err != nil {
				t.Fatal(err)
			}
			a.resolved.model = &model.Model{Name: "test"}
			a.resolved.compactModel = &model.Model{Name: "test"}
			a.builder.AddUserMessage(t.Context(), strings.Repeat("old evidence ", 40000), 40000)
			a.builder.AddAssistantMessage(message.Message{Content: "Latest answer."})
			a.builder.AddBookmark("turn boundary")
			if tc.batch {
				a.builder.AddAssistantMessage(message.Message{
					Calls:  []call.Call{{ID: "large", Name: "Read", Arguments: map[string]any{"text": strings.Repeat("argument ", 40000)}}},
					Tokens: message.Tokens{Total: 40000},
				})
				a.builder.AddBookmark("tool display")
				a.builder.AddToolResult(t.Context(), "Read", "large", "Tool receipt", 3)
			}
			compactions, chats := 0, 0
			a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(req request.Request) (message.Message, usage.Usage, error) {
				if len(req.Tools) == 0 {
					compactions++
					return message.Message{Content: tc.summary}, usage.Usage{}, tc.failure
				}
				chats++
				return message.Message{Content: "Accepted."}, usage.Usage{}, nil
			}}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var err error
			if tc.until {
				_, err = commands.Until().Execute(ctx, a, "not", "todo_empty", "Continue")
			} else {
				err = a.ProcessInput(ctx, "Continue")
			}
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("admission did not terminate on its own: %v", err)
			}
			if (err == nil) != tc.accept || compactions != 1 {
				t.Fatalf("accepted=%t compactions=%d error=%v", tc.accept, compactions, err)
			}
			if tc.failure != nil && !errors.Is(err, tc.failure) {
				t.Fatalf("lost compaction cause: %v", err)
			}
			if tc.accept && chats != 1 || !tc.accept && chats != 0 {
				t.Fatalf("main chats = %d, accepted=%t", chats, tc.accept)
			}
			if !tc.accept && tc.failure == nil && !strings.Contains(err.Error(), "input rejected after compaction") {
				t.Fatalf("missing admission diagnostic: %v", err)
			}
			// A rejected turn must not poison the session: clearing context permits
			// the next input, using the same assistant/provider.
			if !tc.accept {
				if _, err := commands.Clear().Execute(t.Context(), a); err != nil {
					t.Fatal(err)
				}
				if err := a.ProcessInput(t.Context(), "Try again"); err != nil {
					t.Fatalf("session could not recover: %v", err)
				}
				if chats != 1 {
					t.Fatalf("recovery chats = %d, want 1", chats)
				}
			}
		})
	}
}

func TestInputAdmissionNoCompactor(t *testing.T) {
	a := selectionAssistant(t)
	a.cfg.Features.Compaction.Agent = ""
	a.cfg.Features.Compaction.Prompt = ""
	a.builder.AddUserMessage(t.Context(), "Oversized evidence", 40000)
	if added, err := a.addUserInput(t.Context(), "Continue"); added || err == nil {
		t.Fatalf("added=%t error=%v", added, err)
	}
	if added, err := a.addUserInput(t.Context(), " \n "); added || err != nil {
		t.Fatalf("blank input should remain a no-op: added=%t error=%v", added, err)
	}
}
