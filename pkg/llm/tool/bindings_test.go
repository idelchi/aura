package tool

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// bindingProbe retains a shared schema so accidental schema mutation is visible.
type bindingProbe struct {
	Base          // ordinary tool defaults
	schema Schema // original model and implementation contract
}

// Name identifies the harmless fixture tool.
func (p *bindingProbe) Name() string { return "Probe" }

// Schema returns the deliberately shared schema under test.
func (p *bindingProbe) Schema() Schema { return p.schema }

// Execute exposes the actual arguments without external effects.
func (p *bindingProbe) Execute(_ context.Context, args map[string]any) (string, error) {
	return fmt.Sprint(args["project"]), nil
}

// Sandboxable verifies optional interfaces survive preflight unwrapping.
func (p *bindingProbe) Sandboxable() bool { return false }

// TestBindingsContract covers schema visibility, enforcement, validation and isolation.
func TestBindingsContract(t *testing.T) {
	p := &bindingProbe{schema: Schema{Name: "Probe", Parameters: Parameters{Type: "object", Properties: map[string]Property{
		"project": {Type: "string"}, "limit": {Type: "integer"}, "enabled": {Type: "boolean"}, "options": {Type: "object"},
	}, Required: []string{"project", "limit"}}}}
	bindings := Bindings{"Probe": {"project": "fixed", "enabled": false, "options": map[string]any{"a": "original"}}}
	bound, err := bindings.Bind(Tools{p})
	if err != nil {
		t.Fatal(err)
	}
	schema := bound[0].Schema()
	if len(schema.Parameters.Properties) != 1 || !reflect.DeepEqual(schema.Parameters.Required, []string{"limit"}) {
		t.Fatalf("fixed inputs leaked: %+v", schema)
	}
	if len(p.Schema().Parameters.Properties) != 4 || len(p.Schema().Parameters.Required) != 2 {
		t.Fatal("original schema mutated")
	}
	caller := map[string]any{"limit": 2, "project": "override"}
	original, args, err := Prepare(bound[0], caller)
	if err != nil {
		t.Fatal(err)
	}
	if original != p || args["project"] != "fixed" || args["enabled"] != false || caller["project"] != "override" {
		t.Fatal(args, caller)
	}
	if original.(SandboxOverride).Sandboxable() {
		t.Fatal("lost sandbox override")
	}
	args["options"].(map[string]any)["a"] = "mutated"
	_, fresh, _ := Prepare(bound[0], caller)
	if fresh["options"].(map[string]any)["a"] != "original" {
		t.Fatal("shared nested binding")
	}
	if _, _, err := Prepare(bound[0], map[string]any{}); err == nil {
		t.Fatal("missing model argument accepted")
	}
	if _, _, err := Prepare(bound[0], map[string]any{"limit": "bad"}); err == nil {
		t.Fatal("bad model type accepted")
	}
	for _, fixed := range []map[string]any{{"missing": true}, {"limit": "bad"}} {
		if _, err := (Bindings{"Probe": fixed}).Bind(Tools{p}); err == nil {
			t.Fatal("invalid binding accepted", fixed)
		}
	}
	if output, err := bound[0].Execute(t.Context(), caller); err != nil || output != "fixed" {
		t.Fatal(output, err)
	}
	cleared, err := (Bindings{}).Bind(bound)
	if err != nil || cleared[0] != p {
		t.Fatal("rebinding empty config did not unwrap", err)
	}
}

// TestBindingsExpand preserves nil inheritance and structured value types.
func TestBindingsExpand(t *testing.T) {
	var inherited Bindings
	if got, err := inherited.Expand(func(s string) (string, error) { return s, nil }); err != nil || got != nil {
		t.Fatal(got, err)
	}
	input := Bindings{"Probe": {"project": "RUN", "options": map[string]any{"list": []any{"RUN", 2, false}}}}
	got, err := input.Expand(func(s string) (string, error) { return strings.ReplaceAll(s, "RUN", "resolved"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if got["Probe"]["project"] != "resolved" || input["Probe"]["project"] != "RUN" {
		t.Fatal(got, input)
	}
	want := []any{"resolved", 2, false}
	if !reflect.DeepEqual(got["Probe"]["options"].(map[string]any)["list"], want) {
		t.Fatal(got)
	}
}
