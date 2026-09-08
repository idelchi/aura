package assistant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/config/override"
	"github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/tools/assemble"
	"github.com/idelchi/aura/internal/ui"
)

// selectionAssistant constructs a real configured assistant with a harmless interpreted plugin.
func selectionAssistant(t *testing.T) *Assistant {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".aura")
	if err := os.CopyFS(home, os.DirFS("../../tests/fixtures/base/.aura")); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(home, "plugins/selection"), os.DirFS("../plugins/testdata/selection")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"config/agents/disabled.md": heredoc.Doc(`
			---
			name: Disabled
			model:
			  provider: ollama
			  name: gpt-oss:20b
			  context: 32768
			mode: Edit
			system: Test
			features:
			  plugins:
			    exclude: ["*"]
			---
		`),
		"config/modes/disabled.md": heredoc.Doc(`
			---
			name: Disabled
			features:
			  plugins:
			    exclude: ["*"]
			---
		`),
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(home, path), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	opts := config.Options{Homes: []string{home}, WriteHome: home, LaunchDir: filepath.Dir(home), WorkDir: filepath.Dir(home), WithPlugins: true}
	cfg, paths, err := config.New(opts, config.AllParts()...)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := plugins.LoadAll(cfg.Plugins, cfg.Features.PluginConfig, home)
	if err != nil {
		t.Fatal(err)
	}
	rt := &config.Runtime{WithPlugins: true}
	list := todo.New()
	events := make(chan ui.Event, 1000)
	if _, err := assemble.Tools(assemble.Params{Config: cfg, Paths: paths, Runtime: rt, TodoList: list, Events: events, PluginCache: cache}); err != nil {
		t.Fatal(err)
	}
	ag, err := agent.New(cfg, paths, rt, "Test")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Params{Config: cfg, Paths: paths, Runtime: rt, Agent: ag, Todo: list, Events: events, ConfigOpts: opts, Plugins: cache, Slash: slash.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	if err := a.RebuildState(); err != nil {
		t.Fatal(err)
	}
	return a
}

// assertSelection checks execution and presentation of all plugin exports together.
func assertSelection(t *testing.T, a *Assistant, enabled bool) {
	t.Helper()
	_, command := a.slashRegistry.Lookup("/selection-probe")
	tool := false
	for _, candidate := range a.agent.Tools {
		if candidate.Name() == "SelectionProbe" {
			tool = true
		}
	}
	hooks := a.tools.injectors.RunBeforeChat(t.Context(), a.InjectorState())
	if tool != enabled || command != enabled || (len(hooks) > 0) != enabled {
		t.Fatalf("enabled=%v tool=%v command=%v hooks=%d summary=%s", enabled, tool, command, len(hooks), a.PluginSummary())
	}
	if enabled {
		output, handled, _, err := a.slashRegistry.Handle(t.Context(), a, "/selection-probe")
		if err != nil || !handled || !strings.Contains(output, "selection command ran") {
			t.Fatalf("command: %q %v", output, err)
		}
	}
}

// TestPluginSelectionLifecycle covers agent/mode/task/CLI layers, reload, failover and UI hints.
func TestPluginSelectionLifecycle(t *testing.T) {
	a := selectionAssistant(t)
	hints := a.slashRegistry.HintFor
	assertSelection(t, a, true)
	if err := a.SwitchAgent("Disabled", "task"); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, false)
	if hints("/selection-probe") != "" {
		t.Fatal("UI retained excluded command hints")
	}
	if err := a.reloadConfig(nil); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, false)
	if err := a.SwitchAgent("Test", "user"); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, true)
	if hints("/selection-probe") == "" {
		t.Fatal("UI did not regain command hints")
	}
	if err := a.SwitchMode("Disabled"); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, false)
	if err := a.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, false)
	if err := a.SwitchMode("Edit"); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, true)
	if err := a.setAgentFailover("Disabled"); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, false)
	if err := a.SwitchAgent("Test", "user"); err != nil {
		t.Fatal(err)
	}
	if err := a.MergeFeatures(config.Features{PluginConfig: config.PluginConfig{Exclude: []string{"*"}}}); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, false)
	var target config.OverrideTarget
	nodes, err := override.Cache(&target, []string{"features.plugins.exclude=[]"})
	if err != nil {
		t.Fatal(err)
	}
	a.overrideNodes = nodes
	if err := a.RebuildState(); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, true)
	if err := a.reloadConfig(nil); err != nil {
		t.Fatal(err)
	}
	assertSelection(t, a, true)
}
