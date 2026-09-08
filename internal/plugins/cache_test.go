package plugins

import (
	"github.com/idelchi/aura/internal/config"
	"path/filepath"
	"testing"
)

// TestCacheSelection covers lazy loading, all export types, state retention and failure atomicity.
func TestCacheSelection(t *testing.T) {
	t.Parallel()
	source, err := filepath.Abs("testdata/selection/plugin.yaml")
	if err != nil {
		t.Fatal(err)
	}
	definitions := config.StringCollection[config.Plugin]{
		"selection": {Name: "selection", Source: source},
		"broken":    {Name: "broken", Source: filepath.Join(t.TempDir(), "plugin.yaml")},
		"disabled":  {Name: "disabled", Source: source, Disabled: new(true)},
	}
	none := config.PluginConfig{Exclude: []string{"*"}}
	selected := config.PluginConfig{Include: []string{"selection"}}
	c, err := LoadAll(definitions, none, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c == nil || len(c.loaded) != 0 {
		t.Fatal("excluded plugin code was loaded")
	}
	changed, err := c.Select(selected)
	if err != nil || !changed {
		t.Fatalf("selection: %v %v", changed, err)
	}
	if len(c.Tools()) != 1 || len(c.Hooks()) != 1 || len(c.Commands()) != 1 {
		t.Fatal("missing plugin exports")
	}
	first := c.loaded["selection"]
	if changed, err := c.Select(selected); err != nil || changed {
		t.Fatalf("unchanged selection reloaded: %v %v", changed, err)
	}
	if _, err := c.Select(none); err != nil {
		t.Fatal(err)
	}
	if len(c.Tools())+len(c.Hooks())+len(c.Commands()) != 0 {
		t.Fatal("excluded exports remained active")
	}
	if _, err := c.Select(selected); err != nil {
		t.Fatal(err)
	}
	if c.loaded["selection"] != first {
		t.Fatal("reactivation reset interpreter state")
	}
	if _, err := c.Select(config.PluginConfig{}); err == nil {
		t.Fatal("broken plugin should fail to load")
	}
	if len(c.active) != 1 || c.active[0] != first {
		t.Fatal("failed selection damaged active set")
	}
	selected.Config.Local = map[string]map[string]any{"selection": {"setting": "changed"}}
	if _, err := c.Select(selected); err != nil {
		t.Fatal(err)
	}
	if c.loaded["selection"] == first {
		t.Fatal("changed initialization config did not reload")
	}
	if _, err := c.Select(config.PluginConfig{Include: []string{"disabled"}}); err != nil {
		t.Fatal(err)
	}
	if len(c.active) != 0 {
		t.Fatal("disabled plugin was enabled by include filter")
	}
}
