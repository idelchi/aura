package assistant

import (
	"testing"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/session"
)

// TestResumeInvocationSelection retains initial overrides without tying precedence to a reason label.
func TestResumeInvocationSelection(t *testing.T) {
	a := selectionAssistant(t)
	name, model, provider := "Test", "requested-model", "ollama"
	a.invocationOverrides = agent.Overrides{Agent: &name, Model: &model, Provider: &provider}
	warnings := a.ResumeSession(t.Context(), &session.Session{Meta: session.Meta{Agent: "Disabled", Model: "stored-model", Provider: provider}})
	if len(warnings) != 0 {
		t.Fatalf("resume warnings: %v", warnings)
	}
	if got := a.Resolved(); got.Agent != name || got.Model != model || got.Provider != provider {
		t.Fatalf("initial selection lost during resume: %+v", got)
	}
	// Even a caller's diagnostic label "task" cannot change switch semantics.
	if err := a.SwitchAgent("Disabled", "task"); err != nil {
		t.Fatal(err)
	}
	if a.Resolved().Model != "gpt-oss:20b" {
		t.Fatal("later switch retained the invocation model")
	}
}

// TestTemplateSelectionBeforeInference exposes configured metadata without loading a model.
func TestTemplateSelectionBeforeInference(t *testing.T) {
	a := selectionAssistant(t)
	name, model, provider := "Test", "requested-model", "ollama"
	url := a.cfg.Providers.Get(provider).URL
	if err := a.switchAgent(name, "test", agent.Overrides{Model: &model, Provider: &provider}); err != nil {
		t.Fatal(err)
	}
	data := a.TemplateData()
	if a.resolved.model != nil || data.Model.Name != model || data.Provider.Name != provider || data.Provider.URL != url {
		t.Fatalf("wrong pre-inference context: model=%+v provider=%+v", data.Model, data.Provider)
	}
	if err := a.SwitchAgent("Disabled", "user"); err != nil {
		t.Fatal(err)
	}
	if a.TemplateData().Model.Name != "gpt-oss:20b" {
		t.Fatal("template context did not follow agent selection")
	}
}
