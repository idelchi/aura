package assistant

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/slash/commands"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

// toolFilterProvider supplies deterministic responses while retaining real model configuration.
type toolFilterProvider struct {
	providers.Provider                                       // delegates unrelated provider behavior
	respond            func(request.Request) message.Message // records schemas and controls attempted calls
}

// Chat returns the next response without contacting an inference service.
func (p toolFilterProvider) Chat(_ context.Context, req request.Request, _ stream.Func) (message.Message, usage.Usage, error) {
	return p.respond(req), usage.Usage{}, nil
}

// TestToolFilterHookLifecycle checks actual plugin hooks, request schemas, dispatch and next-turn reset.
func TestToolFilterHookLifecycle(t *testing.T) {
	for _, phase := range []string{"response", "tool", "text-only"} {
		t.Run(phase, func(t *testing.T) {
			a := selectionAssistant(t)
			if err := os.CopyFS(filepath.Join(a.configOpts.WriteHome, "plugins/turn-filter"), os.DirFS("../plugins/testdata/turn-filter")); err != nil {
				t.Fatal(err)
			}
			if err := a.Reload(t.Context()); err != nil {
				t.Fatal(err)
			}
			a.resolved.model = &model.Model{Name: "test"}
			requests := 0
			a.agent.Provider = toolFilterProvider{Provider: a.agent.Provider, respond: func(req request.Request) message.Message {
				requests++
				advertised := slices.ContainsFunc(req.Tools, func(s tool.Schema) bool { return s.Name == "SelectionProbe" })
				if advertised != (requests == 1) {
					t.Fatalf("request %d: probe advertised=%v", requests, advertised)
				}
				if requests == 1 {
					response := message.Message{Content: "restrict probe"}
					if phase != "text-only" {
						response.Calls = []call.Call{{ID: "first", Name: "SelectionProbe", Arguments: map[string]any{}}}
					}
					if phase == "tool" {
						response.Content = ""
					}
					return response
				}
				if phase == "tool" && requests == 2 {
					// A model may call a tool that is no longer advertised.
					return message.Message{Calls: []call.Call{{ID: "repeat", Name: "SelectionProbe", Arguments: map[string]any{}}}}
				}
				return message.Message{Content: "Assessment complete."}
			}}
			if err := a.ProcessInput(t.Context(), "Assess the fixture"); err != nil {
				t.Fatal(err)
			}
			wantCalls, wantRequests := 0, 2
			if phase == "tool" {
				wantCalls, wantRequests = 1, 3
			} else if phase == "text-only" {
				wantRequests = 1
			}
			if got := a.session.callLimits.Snapshot()["SelectionProbe"].Attempts; got != wantCalls || requests != wantRequests {
				t.Fatalf("executions=%d requests=%d, want %d/%d", got, requests, wantCalls, wantRequests)
			}
			if slices.Contains(a.ToolNames(), "SelectionProbe") || slices.Contains(a.InjectorState().AvailableTools, "SelectionProbe") {
				t.Fatal("restricted tool remains in assistant/plugin catalog")
			}
			probe, err := a.agent.Tools.Get("SelectionProbe")
			if err != nil {
				t.Fatal(err)
			}
			for name, execute := range map[string]func() error{
				"batch":    func() error { _, err := a.ExecuteSubTool(t.Context(), "SelectionProbe", nil); return err },
				"delegate": func() error { _, err := a.subagentExecuteOverride(nil)(t.Context(), probe, nil); return err },
				"sandbox": func() error {
					a.toggles.sandbox = true
					defer func() { a.toggles.sandbox = false }()
					_, err := a.executeTool(t.Context(), probe, nil)
					return err
				},
			} {
				if err := execute(); err == nil || !strings.Contains(err.Error(), "disabled for this turn") {
					t.Fatalf("%s bypassed turn restriction: %v", name, err)
				}
			}
			a.agent.Provider = toolFilterProvider{Provider: a.agent.Provider, respond: func(req request.Request) message.Message {
				if !slices.ContainsFunc(req.Tools, func(s tool.Schema) bool { return s.Name == "SelectionProbe" }) {
					t.Fatal("fresh user turn did not restore tool availability")
				}
				return message.Message{Content: "New assessment complete."}
			}}
			if err := a.ProcessInput(t.Context(), "Start another assessment"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestToolFiltersCompose checks that later hook phases cannot widen an earlier restriction.
func TestToolFiltersCompose(t *testing.T) {
	a := selectionAssistant(t)
	filters := []injector.Injection{
		{Tools: &config.Tools{Enabled: []string{"SelectionProbe", "Bash"}}},
		{Tools: &config.Tools{Disabled: []string{"Selection*"}}},
		{Tools: &config.Tools{Enabled: []string{"SelectionProbe", "Bash", "Read"}}},
	}
	if a.injectMessages(filters) {
		t.Fatal("silent restrictions emitted a continuation message")
	}
	if got := a.loop.filterTools(a.agent.Tools).Names(); !slices.Equal(got, []string{"Bash"}) {
		t.Fatalf("filters widened: %v", got)
	}
	a.injectMessages(filters)
	if len(a.loop.toolsFilters) != len(filters) {
		t.Fatal("identical restrictions accumulated repeatedly")
	}
	if err := a.SwitchAgent("Test", "user"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(a.ToolNames(), "SelectionProbe") {
		t.Fatal("agent rebuild discarded this turn's restriction")
	}
	if _, err := commands.Clear().Execute(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(a.ToolNames(), "SelectionProbe") {
		t.Fatal("/new retained the previous turn's restriction")
	}
}
