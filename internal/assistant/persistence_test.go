package assistant

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// eventLog collects every event the assistant emits.
type eventLog struct {
	mu     sync.Mutex
	events []ui.Event
}

// recordEvents redirects the assistant's events into a log until the test ends.
func recordEvents(t *testing.T, a *Assistant) *eventLog {
	t.Helper()

	ch := make(chan ui.Event, 4096)
	a.events = ch

	log := &eventLog{}
	done := make(chan struct{})

	go func() {
		defer close(done)

		for event := range ch {
			log.mu.Lock()
			log.events = append(log.events, event)
			log.mu.Unlock()
		}
	}()

	t.Cleanup(func() {
		close(ch)
		<-done
	})

	return log
}

// persisted returns the SessionPersisted events recorded so far.
func (l *eventLog) persisted() []ui.SessionPersisted {
	l.mu.Lock()
	defer l.mu.Unlock()

	var out []ui.SessionPersisted

	for _, e := range l.events {
		if p, ok := e.(ui.SessionPersisted); ok {
			out = append(out, p)
		}
	}

	return out
}

// waitFor polls the log until match finds an event or the deadline passes.
func (l *eventLog) waitFor(t *testing.T, match func(ui.Event) bool) ui.Event {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		l.mu.Lock()
		for _, e := range l.events {
			if match(e) {
				l.mu.Unlock()

				return e
			}
		}
		l.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("event not observed")

	return nil
}

// assertPairs fails unless every assistant call has exactly one result and every result answers a prior call.
func assertPairs(t *testing.T, msgs message.Messages) {
	t.Helper()

	open := map[string]bool{}

	for _, msg := range msgs {
		for _, c := range msg.Calls {
			open[c.ID] = true
		}

		if msg.Role == roles.Tool {
			if !open[msg.ToolCallID] {
				t.Fatalf("tool result %q has no preceding call: %+v", msg.ToolCallID, msgs)
			}

			delete(open, msg.ToolCallID)
		}
	}

	if len(open) > 0 {
		t.Fatalf("calls without results %v in persisted history: %+v", open, msgs)
	}
}

// storedText returns the session file content for id.
func storedText(t *testing.T, dir, id string) string {
	t.Helper()

	data, err := os.ReadFile(session.NewStore(dir).Path(id).Path())
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// Each iteration's tool results are on disk before the next model request.
func TestCheckpointEveryIteration(t *testing.T) {
	dir := t.TempDir()
	a := sessionAssistant(t, dir)
	log := recordEvents(t, a)

	chats := 0
	a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(_ request.Request) (message.Message, usage.Usage, error) {
		chats++

		switch chats {
		case 1:
			return message.Message{Calls: []call.Call{{ID: "p1", Name: "SelectionProbe", Arguments: map[string]any{}}}}, usage.Usage{}, nil
		case 2:
			id := a.session.manager.ActiveID()
			if !strings.Contains(storedText(t, dir, id), `"p1"`) {
				t.Error("first iteration was not checkpointed before the second request")
			}

			return message.Message{Calls: []call.Call{{ID: "p2", Name: "SelectionProbe", Arguments: map[string]any{}}}}, usage.Usage{}, nil
		default:
			return message.Message{Content: "done"}, usage.Usage{}, nil
		}
	}}

	if err := a.ProcessInput(t.Context(), "work"); err != nil {
		t.Fatal(err)
	}

	log.waitFor(t, func(e ui.Event) bool {
		p, ok := e.(ui.SessionPersisted)

		return ok && p.Reason == ui.PersistRun
	})

	var reasons []ui.PersistReason
	for _, p := range log.persisted() {
		reasons = append(reasons, p.Reason)
	}

	if len(reasons) != 3 || reasons[0] != ui.PersistCheckpoint || reasons[1] != ui.PersistCheckpoint || reasons[2] != ui.PersistRun {
		t.Fatalf("persist reasons = %v, want two checkpoints then run", reasons)
	}

	saved := savedSessions(t, dir)
	if len(saved) != 1 || !hasToolResult(saved[0].Messages, "p2", "selection tool ran") || !hasMessage(saved[0].Messages, roles.Assistant, "done") {
		t.Fatalf("final session incomplete: %+v", saved)
	}

	if last := log.persisted()[2]; last.Revision != a.session.manager.ActiveRevision() || last.Messages != saved[0].ConversationLength() {
		t.Fatalf("run persist event %+v does not describe the stored session", last)
	}

	assertPairs(t, saved[0].Messages)
}

// Cancelling while tools run still pairs every call with a result in the saved history.
func TestCancelledToolsPersistCompletePairs(t *testing.T) {
	dir := t.TempDir()
	a := sessionAssistant(t, dir)
	recordEvents(t, a)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	chats := 0
	a.agent.Provider = contextProbe{compactionRequestProbe{Provider: a.agent.Provider}, func(ctx context.Context, _ request.Request) (message.Message, usage.Usage, error) {
		chats++
		if chats > 1 {
			<-ctx.Done()

			return message.Message{}, usage.Usage{}, ctx.Err()
		}

		time.AfterFunc(500*time.Millisecond, cancel)

		return message.Message{Calls: []call.Call{
			{ID: "slow", Name: "Bash", Arguments: map[string]any{"command": "sleep 30"}},
			{ID: "fast", Name: "SelectionProbe", Arguments: map[string]any{}},
		}}, usage.Usage{}, nil
	}}

	start := time.Now()

	err := a.ProcessInput(ctx, "run both")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ProcessInput = %v, want context.Canceled", err)
	}

	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}

	saved := savedSessions(t, dir)
	if len(saved) != 1 {
		t.Fatalf("saved %d sessions, want 1", len(saved))
	}

	assertPairs(t, saved[0].Messages)

	if !hasToolResult(saved[0].Messages, "fast", "selection tool ran") {
		t.Fatalf("completed call lost its result: %+v", saved[0].Messages)
	}

	for _, msg := range saved[0].Messages {
		if msg.Role == roles.Tool && msg.ToolCallID == "slow" && !strings.Contains(msg.Content, "cancel") {
			t.Fatalf("interrupted call result = %q, want a cancellation error", msg.Content)
		}
	}
}

// contextProbe is a probe provider whose chat observes the request context.
type contextProbe struct {
	compactionRequestProbe
	chat func(context.Context, request.Request) (message.Message, usage.Usage, error)
}

func (p contextProbe) Chat(ctx context.Context, req request.Request, _ stream.Func) (message.Message, usage.Usage, error) {
	return p.chat(ctx, req)
}

// Rejected input never reaches the saved conversation.
func TestRejectedInputSavesNoPhantom(t *testing.T) {
	dir := t.TempDir()
	a := sessionAssistant(t, dir)
	recordEvents(t, a)

	// The per-turn reload re-reads configuration: remove the compaction agent on disk.
	compaction := filepath.Join(a.paths.Home, "config/features/compaction.yaml")
	if err := os.WriteFile(compaction, []byte("compaction:\n  threshold: 80\n  keep_last_messages: 10\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(_ request.Request) (message.Message, usage.Usage, error) {
		return message.Message{Content: "ok"}, usage.Usage{}, nil
	}}

	if err := a.ProcessInput(t.Context(), "first"); err != nil {
		t.Fatal(err)
	}

	a.builder.AddUserMessage(t.Context(), "Oversized evidence", 40000)

	if err := a.ProcessInput(t.Context(), "Continue"); !errors.Is(err, ErrInputRejected) {
		t.Fatalf("ProcessInput = %v, want ErrInputRejected", err)
	}

	saved := savedSessions(t, dir)
	if len(saved) != 1 || !hasMessage(saved[0].Messages, roles.User, "first") || hasMessage(saved[0].Messages, roles.User, "Continue") {
		t.Fatalf("saved history after rejection: %+v", saved)
	}
}

// A resume that cannot restore the stored state fails and leaves nothing that could overwrite it.
func TestResumeFailureLeavesStoredSessionUntouched(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)

	if _, err := store.Save(&session.Session{
		ID:       "stored",
		Messages: message.New(message.Message{Role: roles.User, Content: "kept"}),
		Meta:     session.Meta{Agent: "NoSuchAgent"},
	}); err != nil {
		t.Fatal(err)
	}

	before := storedText(t, dir, "stored")

	a := sessionAssistant(t, dir)
	recordEvents(t, a)

	sess, err := a.session.manager.Resume("stored")
	if err != nil {
		t.Fatal(err)
	}

	if err := a.ResumeSession(t.Context(), sess); err == nil || !strings.Contains(err.Error(), "NoSuchAgent") {
		t.Fatalf("ResumeSession = %v, want agent restore failure", err)
	}

	if id := a.session.manager.ActiveID(); id != "" {
		t.Fatalf("assistant still attached to %q after failed resume", id)
	}

	if err := a.AutoSave(); err != nil {
		t.Fatal(err)
	}

	if storedText(t, dir, "stored") != before {
		t.Fatal("failed resume rewrote the stored session")
	}
}

// Starting a new session forgets reads and loaded tools, and detaches from the stored session.
func TestNewSessionResetsSessionState(t *testing.T) {
	dir := t.TempDir()
	a := sessionAssistant(t, dir)
	recordEvents(t, a)

	a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(_ request.Request) (message.Message, usage.Usage, error) {
		return message.Message{Content: "ok"}, usage.Usage{}, nil
	}}

	if err := a.ProcessInput(t.Context(), "first"); err != nil {
		t.Fatal(err)
	}

	first := a.session.manager.ActiveID()
	before := storedText(t, dir, first)

	a.tracker.RecordRead("/tmp/somewhere")
	a.tools.loaded["Deferred"] = true

	if err := a.NewSession(t.Context()); err != nil {
		t.Fatal(err)
	}

	if a.tracker.WasRead("/tmp/somewhere") || len(a.tools.loaded) != 0 || a.session.manager.ActiveID() != "" || len(a.builder.History()) != 1 {
		t.Fatal("new session kept state from the previous one")
	}

	if err := a.ProcessInput(t.Context(), "second"); err != nil {
		t.Fatal(err)
	}

	if a.session.manager.ActiveID() == first || storedText(t, dir, first) != before {
		t.Fatal("new session overwrote the previous session")
	}
}

// The interactive loop correlates identified inputs with admission, completion and persistence.
func TestLoopCorrelatesIdentifiedInputs(t *testing.T) {
	dir := t.TempDir()
	a := sessionAssistant(t, dir)
	log := recordEvents(t, a)

	a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(_ request.Request) (message.Message, usage.Usage, error) {
		return message.Message{Content: "answer"}, usage.Usage{}, nil
	}}

	inputs := make(chan ui.UserInput)
	ctx, cancel := context.WithCancel(t.Context())

	loopDone := make(chan error, 1)
	go func() { loopDone <- a.Loop(ctx, inputs, nil, nil) }()

	t.Cleanup(func() {
		cancel()
		<-loopDone
	})

	inputs <- ui.UserInput{ID: "r1", Kind: ui.InputPrompt, Text: "/help is just text here"}

	log.waitFor(t, func(e ui.Event) bool {
		p, ok := e.(ui.SessionPersisted)

		return ok && p.Reason == ui.PersistRun && len(p.InputIDs) == 1 && p.InputIDs[0] == "r1"
	})

	log.mu.Lock()
	var admitted, done bool
	for _, e := range log.events {
		switch ev := e.(type) {
		case ui.InputAdmitted:
			admitted = admitted || ev.InputID == "r1"
		case ui.AssistantDone:
			done = done || (len(ev.InputIDs) == 1 && ev.InputIDs[0] == "r1" && ev.Error == nil)
		}
	}
	log.mu.Unlock()

	if !admitted || !done {
		t.Fatalf("admitted=%v done=%v", admitted, done)
	}

	if !hasMessage(a.builder.History(), roles.User, "/help is just text here") {
		t.Fatal("prompt input was not treated as a user message")
	}

	inputs <- ui.UserInput{ID: "r2", Kind: ui.InputCommand, Text: "not a command"}

	handled := log.waitFor(t, func(e ui.Event) bool {
		h, ok := e.(ui.SlashCommandHandled)

		return ok && h.InputID == "r2"
	}).(ui.SlashCommandHandled)

	if !errors.Is(handled.Error, ErrNotCommand) || handled.Forwarded {
		t.Fatalf("command input = %+v, want ErrNotCommand", handled)
	}

	log.waitFor(t, func(e ui.Event) bool {
		p, ok := e.(ui.SessionPersisted)

		return ok && p.Reason == ui.PersistCommand && len(p.InputIDs) == 1 && p.InputIDs[0] == "r2"
	})
}
