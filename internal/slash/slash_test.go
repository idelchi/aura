package slash_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/conversation"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/snapshot"
	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// stubContext is a minimal implementation of slash.Context.
// Every method panics with "not implemented" except EventChan, which returns
// a real buffered channel so Handle can send CommandResult events without blocking.
type stubContext struct {
	events chan ui.Event
}

func newStubContext() *stubContext {
	return &stubContext{events: make(chan ui.Event, 16)}
}

func (s *stubContext) EventChan() chan<- ui.Event { return s.events }

// --- all remaining Context methods panic; they are never called in these tests ---

func (s *stubContext) SwitchAgent(_, _ string) error                    { panic("not implemented") }
func (s *stubContext) SwitchMode(_ string) error                        { panic("not implemented") }
func (s *stubContext) SwitchModel(_ context.Context, _, _ string) error { panic("not implemented") }
func (s *stubContext) SetThink(_ thinking.Value) error                  { panic("not implemented") }
func (s *stubContext) SetSandbox(_ bool) error                          { panic("not implemented") }
func (s *stubContext) ResizeContext(_ int) error                        { panic("not implemented") }
func (s *stubContext) SetAuto(_ bool)                                   { panic("not implemented") }
func (s *stubContext) ResetTokens()                                     { panic("not implemented") }
func (s *stubContext) Compact(_ context.Context, _ bool) error          { panic("not implemented") }
func (s *stubContext) GenerateTitle(_ context.Context) (string, error)  { panic("not implemented") }
func (s *stubContext) ProcessInput(_ context.Context, _ string) error   { panic("not implemented") }
func (s *stubContext) Reload(_ context.Context) error                   { panic("not implemented") }
func (s *stubContext) ResumeSession(_ context.Context, _ *session.Session) []string {
	panic("not implemented")
}
func (s *stubContext) Resolved() config.Resolved                        { panic("not implemented") }
func (s *stubContext) Status() ui.Status                                 { panic("not implemented") }
func (s *stubContext) DisplayHints() ui.DisplayHints                     { panic("not implemented") }
func (s *stubContext) SandboxDisplay() string                            { panic("not implemented") }
func (s *stubContext) SystemPrompt() string                              { panic("not implemented") }
func (s *stubContext) ToolNames() []string                               { panic("not implemented") }
func (s *stubContext) LoadedTools() []string                             { panic("not implemented") }
func (s *stubContext) ResolvedModel() model.Model                        { panic("not implemented") }
func (s *stubContext) ToolPolicy() *config.ToolPolicy                    { panic("not implemented") }
func (s *stubContext) Cfg() config.Config                                { panic("not implemented") }
func (s *stubContext) Paths() config.Paths                               { panic("not implemented") }
func (s *stubContext) Runtime() *config.Runtime                          { panic("not implemented") }
func (s *stubContext) Builder() *conversation.Builder                    { panic("not implemented") }
func (s *stubContext) SessionManager() *session.Manager                  { panic("not implemented") }
func (s *stubContext) TodoList() *todo.List                              { panic("not implemented") }
func (s *stubContext) SessionStats() *stats.Stats                        { panic("not implemented") }
func (s *stubContext) InjectorRegistry() *injector.Registry              { panic("not implemented") }
func (s *stubContext) MCPSessions() []*mcp.Session                       { panic("not implemented") }
func (s *stubContext) RegisterMCPSession(_ *mcp.Session) error           { panic("not implemented") }
func (s *stubContext) SnapshotManager() *snapshot.Manager                { panic("not implemented") }
func (s *stubContext) RequestExit()                                      { panic("not implemented") }
func (s *stubContext) SetVerbose(_ bool)                                 { panic("not implemented") }
func (s *stubContext) SetDone(_ bool) error                              { panic("not implemented") }
func (s *stubContext) ReadBeforePolicy() tool.ReadBeforePolicy           { return tool.DefaultReadBeforePolicy() }
func (s *stubContext) SetReadBeforePolicy(_ tool.ReadBeforePolicy) error { return nil }
func (s *stubContext) ModelListCache() []slash.ProviderModels            { panic("not implemented") }
func (s *stubContext) CacheModelList(_ []slash.ProviderModels)           { panic("not implemented") }
func (s *stubContext) ClearModelListCache()                              { panic("not implemented") }
func (s *stubContext) SessionMeta() session.Meta                         { panic("not implemented") }
func (s *stubContext) PluginSummary() string                             { panic("not implemented") }
func (s *stubContext) TemplateVars() map[string]string                   { return nil }

// drainEvents reads all buffered events from the stub's channel without blocking.
func drainEvents(s *stubContext) []ui.Event {
	ch := s.events

	var out []ui.Event

	for {
		select {
		case e := <-ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

// TestRegistry_RegisterAndLookup verifies that registered commands can be
// found by name (case-insensitive) and that missing names return false.
func TestRegistry_RegisterAndLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		lookup    string
		wantFound bool
		wantName  string
	}{
		{
			name:      "exact name matches",
			lookup:    "/greet",
			wantFound: true,
			wantName:  "/greet",
		},
		{
			name:      "uppercase lookup matches",
			lookup:    "/GREET",
			wantFound: true,
			wantName:  "/greet",
		},
		{
			name:      "mixed case lookup matches",
			lookup:    "/Greet",
			wantFound: true,
			wantName:  "/greet",
		},
		{
			name:      "unknown command returns false",
			lookup:    "/unknown",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := slash.New(slash.Command{
				Name: "/greet",
				Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
					return "hello", nil
				},
			})

			cmd, found := r.Lookup(tt.lookup)
			if found != tt.wantFound {
				t.Errorf("Lookup(%q) found = %v, want %v", tt.lookup, found, tt.wantFound)
			}

			if tt.wantFound && cmd.Name != tt.wantName {
				t.Errorf("Lookup(%q).Name = %q, want %q", tt.lookup, cmd.Name, tt.wantName)
			}
		})
	}
}

// TestRegistry_LookupDoesNotResolveAliases verifies that Lookup returns false
// for alias names — aliases are only resolved inside Handle.
func TestRegistry_LookupDoesNotResolveAliases(t *testing.T) {
	t.Parallel()

	r := slash.New(slash.Command{
		Name:    "/greet",
		Aliases: []string{"/hi"},
		Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
			return "hello", nil
		},
	})

	_, found := r.Lookup("/hi")
	if found {
		t.Errorf("Lookup(%q) = true, want false (aliases are not in the commands map)", "/hi")
	}

	_, found = r.Lookup("/greet")
	if !found {
		t.Errorf("Lookup(%q) = false, want true", "/greet")
	}
}

// TestRegistry_HintFor verifies hint text retrieval and alias resolution.
func TestRegistry_HintFor(t *testing.T) {
	t.Parallel()

	r := slash.New(slash.Command{
		Name:    "/greet",
		Aliases: []string{"/hi"},
		Hints:   "[name]",
		Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
			return "", nil
		},
	})

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "canonical name returns hints",
			input: "/greet",
			want:  "[name]",
		},
		{
			name:  "alias resolves to canonical hints",
			input: "/hi",
			want:  "[name]",
		},
		{
			name:  "unknown command returns empty",
			input: "/unknown",
			want:  "",
		},
		{
			name:  "case insensitive canonical",
			input: "/GREET",
			want:  "[name]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := r.HintFor(tt.input)
			if got != tt.want {
				t.Errorf("HintFor(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestRegistry_Handle covers dispatch, non-slash input, forwarding, and ErrUsage wrapping.
func TestRegistry_Handle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantMsg     string
		wantHandled bool
		wantForward bool
		wantErr     bool
		errContains string
	}{
		{
			name:        "non-slash input is not handled",
			input:       "hello world",
			wantMsg:     "",
			wantHandled: false,
			wantForward: false,
			wantErr:     false,
		},
		{
			name:        "known command dispatches and returns message",
			input:       "/ping",
			wantMsg:     "pong",
			wantHandled: true,
			wantForward: false,
			wantErr:     false,
		},
		{
			name:        "forward command sets forward=true",
			input:       "/fwd",
			wantMsg:     "forwarded",
			wantHandled: true,
			wantForward: true,
			wantErr:     false,
		},
		{
			name:        "unknown command returns error and handled=true",
			input:       "/notregistered",
			wantMsg:     "",
			wantHandled: true,
			wantForward: false,
			wantErr:     true,
			errContains: "unknown command",
		},
		{
			name:        "ErrUsage wraps to usage message",
			input:       "/badusage",
			wantMsg:     "",
			wantHandled: true,
			wantForward: false,
			wantErr:     true,
			errContains: "usage:",
		},
		{
			name:        "wrapped ErrUsage preserves context message",
			input:       "/wrappedusage",
			wantMsg:     "",
			wantHandled: true,
			wantForward: false,
			wantErr:     true,
			errContains: "bad value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := slash.New(
				slash.Command{
					Name:   "/ping",
					Silent: true, // silence so EventChan isn't needed for this subtest
					Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
						return "pong", nil
					},
				},
				slash.Command{
					Name:    "/fwd",
					Forward: true,
					Silent:  true,
					Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
						return "forwarded", nil
					},
				},
				slash.Command{
					Name:   "/badusage",
					Hints:  "<required>",
					Silent: true,
					Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
						return "", slash.ErrUsage
					},
				},
				slash.Command{
					Name:   "/wrappedusage",
					Hints:  "<value>",
					Silent: true,
					Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
						return "", fmt.Errorf("bad value %q: %w", "xyz", slash.ErrUsage)
					},
				},
			)

			ctx := newStubContext()
			msg, handled, forward, err := r.Handle(context.Background(), ctx, tt.input)

			if handled != tt.wantHandled {
				t.Errorf("Handle(%q) handled = %v, want %v", tt.input, handled, tt.wantHandled)
			}

			if forward != tt.wantForward {
				t.Errorf("Handle(%q) forward = %v, want %v", tt.input, forward, tt.wantForward)
			}

			if msg != tt.wantMsg {
				t.Errorf("Handle(%q) msg = %q, want %q", tt.input, msg, tt.wantMsg)
			}

			if tt.wantErr {
				if err == nil {
					t.Errorf("Handle(%q) err = nil, want non-nil", tt.input)
				} else if tt.errContains != "" && !containsStr(err.Error(), tt.errContains) {
					t.Errorf("Handle(%q) err = %q, want to contain %q", tt.input, err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Handle(%q) err = %v, want nil", tt.input, err)
				}
			}
		})
	}
}

// TestRegistry_HandleAlias verifies that an alias in the input dispatches to
// the canonical command's Execute function.
func TestRegistry_HandleAlias(t *testing.T) {
	t.Parallel()

	called := false
	r := slash.New(slash.Command{
		Name:    "/greet",
		Aliases: []string{"/hi"},
		Silent:  true,
		Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
			called = true

			return "hello", nil
		},
	})

	ctx := newStubContext()

	msg, handled, _, err := r.Handle(context.Background(), ctx, "/hi")
	if err != nil {
		t.Fatalf("Handle(%q) unexpected error: %v", "/hi", err)
	}

	if !handled {
		t.Errorf("Handle(%q) handled = false, want true", "/hi")
	}

	if msg != "hello" {
		t.Errorf("Handle(%q) msg = %q, want %q", "/hi", msg, "hello")
	}

	if !called {
		t.Errorf("Handle(%q) did not call the canonical command's Execute", "/hi")
	}
}

// TestRegistry_HandleSendsCommandResultEvent verifies that Handle sends a
// CommandResult event for non-silent commands before dispatching.
func TestRegistry_HandleSendsCommandResultEvent(t *testing.T) {
	t.Parallel()

	r := slash.New(slash.Command{
		Name: "/ping",
		Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
			return "pong", nil
		},
	})

	ctx := newStubContext()

	_, _, _, err := r.Handle(context.Background(), ctx, "/ping")
	if err != nil {
		t.Fatalf("Handle unexpected error: %v", err)
	}

	events := drainEvents(ctx)
	found := false

	for _, e := range events {
		if cr, ok := e.(ui.CommandResult); ok && cr.Command == "/ping" {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Handle did not send a CommandResult event with Command=%q", "/ping")
	}
}

// TestRegistry_HandleSilentNoCommandResultEvent verifies that a Silent command
// does not echo a CommandResult event.
func TestRegistry_HandleSilentNoCommandResultEvent(t *testing.T) {
	t.Parallel()

	r := slash.New(slash.Command{
		Name:   "/quiet",
		Silent: true,
		Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
			return "", nil
		},
	})

	ctx := newStubContext()

	_, _, _, err := r.Handle(context.Background(), ctx, "/quiet")
	if err != nil {
		t.Fatalf("Handle unexpected error: %v", err)
	}

	events := drainEvents(ctx)
	for _, e := range events {
		if _, ok := e.(ui.CommandResult); ok {
			t.Errorf("Handle sent CommandResult event for silent command, want none")
		}
	}
}

// TestRegistry_HandleCommandError verifies that an error returned by Execute
// (that is not ErrUsage) is propagated directly.
func TestRegistry_HandleCommandError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	r := slash.New(slash.Command{
		Name:   "/fail",
		Silent: true,
		Execute: func(_ context.Context, _ slash.Context, _ ...string) (string, error) {
			return "", sentinel
		},
	})

	ctx := newStubContext()
	_, handled, _, err := r.Handle(context.Background(), ctx, "/fail")

	if !handled {
		t.Errorf("Handle(%q) handled = false, want true", "/fail")
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("Handle(%q) err = %v, want %v", "/fail", err, sentinel)
	}
}

// TestRegistry_HandleShlexSplitting verifies that Handle() uses shlex for
// quote-aware argument splitting, falling back to strings.Fields on parse error.
func TestRegistry_HandleShlexSplitting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantArgs  int
		wantFirst string
	}{
		{
			name:      "quoted argument is single token",
			input:     `/echo "hello world"`,
			wantArgs:  1,
			wantFirst: "hello world",
		},
		{
			name:      "mid-token quotes stripped",
			input:     `/echo bash:"go build ./..."`,
			wantArgs:  1,
			wantFirst: "bash:go build ./...",
		},
		{
			name:      "multiple quoted args",
			input:     `/echo "first arg" "second arg"`,
			wantArgs:  2,
			wantFirst: "first arg",
		},
		{
			name:      "apostrophe falls back to Fields",
			input:     `/echo what's wrong`,
			wantArgs:  2,
			wantFirst: "what's",
		},
		{
			name:      "no quotes same as Fields",
			input:     `/echo one two three`,
			wantArgs:  3,
			wantFirst: "one",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var captured []string

			r := slash.New(slash.Command{
				Name:   "/echo",
				Silent: true,
				Execute: func(_ context.Context, _ slash.Context, args ...string) (string, error) {
					captured = args

					return "", nil
				},
			})

			ctx := newStubContext()

			_, _, _, err := r.Handle(context.Background(), ctx, tt.input)
			if err != nil {
				t.Fatalf("Handle(%q) error: %v", tt.input, err)
			}

			if len(captured) != tt.wantArgs {
				t.Errorf(
					"Handle(%q) args count = %d, want %d (args: %q)",
					tt.input,
					len(captured),
					tt.wantArgs,
					captured,
				)
			}

			if len(captured) > 0 && captured[0] != tt.wantFirst {
				t.Errorf("Handle(%q) first arg = %q, want %q", tt.input, captured[0], tt.wantFirst)
			}
		})
	}
}

// containsStr returns true if s contains substr.
// Uses a simple scan to avoid importing strings in the test file unnecessarily,
// and to keep the helper self-contained.
func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
