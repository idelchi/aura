package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/calllimit"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/slash/commands"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// TestCallLimitsExecutionPaths shares allowance across direct, Batch and delegated calls.
func TestCallLimitsExecutionPaths(t *testing.T) {
	a := selectionAssistant(t)
	rules := calllimit.Rules{"SelectionProbe": {Max: 1, Count: "success"}}
	a.cfg.Features.ToolExecution.CallLimits = rules
	a.globalFeatures.ToolExecution.CallLimits = rules
	probe, err := a.agent.Tools.Get("SelectionProbe")
	if err != nil {
		t.Fatal(err)
	}
	if a.IsToolParallel("SelectionProbe") {
		t.Fatal("limited tool can overtake its earlier queued calls")
	}
	a.executeTools(t.Context(), []call.Call{
		{ID: "one", Name: "SelectionProbe", Arguments: map[string]any{}},
		{ID: "two", Name: "SelectionProbe", Arguments: map[string]any{}},
	})
	if got := a.session.callLimits.Snapshot()["SelectionProbe"]; got != (calllimit.Usage{Attempts: 1, Successes: 1}) {
		t.Fatal(got)
	}
	if _, err := a.ExecuteSubTool(t.Context(), "SelectionProbe", nil); err == nil || !strings.Contains(err.Error(), "not executed") {
		t.Fatalf("Batch bypass: %v", err)
	}
	delegated := a.subagentExecuteOverride(nil)
	if _, err := delegated(t.Context(), probe, nil); err == nil {
		t.Fatal("delegation bypass")
	}
	// Agent changes and a new user turn do not reset the conversation's allowance.
	if err := a.SwitchAgent("Test", "user"); err != nil {
		t.Fatal(err)
	}
	a.loop = loopState{}
	if _, err := a.executeTool(t.Context(), probe, nil); err == nil {
		t.Fatal("agent/turn reset allowance")
	}
	// Serialization through session metadata preserves usage without message inspection.
	saved := session.Session{Meta: a.SessionMeta()}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var restored session.Session
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	b := selectionAssistant(t)
	b.cfg.Features.ToolExecution.CallLimits = rules
	b.globalFeatures.ToolExecution.CallLimits = rules
	_ = b.ResumeSession(t.Context(), &restored)
	if _, err := b.executeTool(t.Context(), probe, nil); err == nil {
		t.Fatal("resume reset allowance")
	}
	// /new is /clear, and explicitly starts a fresh allowance.
	if _, err := commands.Clear().Execute(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	if _, err := b.executeTool(t.Context(), probe, nil); err != nil {
		t.Fatal(err)
	}
}

// TestChildCallLimits applies a child's own policy even without a parent limit.
func TestChildCallLimits(t *testing.T) {
	a := selectionAssistant(t)
	probe, err := a.agent.Tools.Get("SelectionProbe")
	if err != nil {
		t.Fatal(err)
	}
	execute := a.subagentExecuteOverride(calllimit.Rules{"SelectionProbe": {Max: 1}})
	if _, err := execute(context.Background(), probe, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(context.Background(), probe, nil); err == nil {
		t.Fatal("child allowance bypassed")
	}
	if a.session.callLimits.Snapshot()["SelectionProbe"].Successes != 1 {
		t.Fatal("parent did not count child execution")
	}
}
