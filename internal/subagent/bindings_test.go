package subagent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// boundProbe reuses the runner fixture while checking arguments seen by its hooks.
type boundProbe struct {
	fakeTool // ordinary hook and path behavior
}

// Schema exposes the required field to be hidden by configuration.
func (p *boundProbe) Schema() tool.Schema {
	return tool.Schema{Name: p.name, Parameters: tool.Parameters{Type: "object", Properties: map[string]tool.Property{"scope": {Type: "string"}}, Required: []string{"scope"}}}
}

// Pre ensures bindings were applied before optional tool hooks.
func (p *boundProbe) Pre(_ context.Context, args map[string]any) error {
	if args["scope"] != "fixed" {
		return fmt.Errorf("hook saw unbound arguments: %v", args)
	}
	return nil
}

// Execute checks dispatch receives the same fixed value as preflight.
func (p *boundProbe) Execute(_ context.Context, args map[string]any) (string, error) {
	p.executed = args["scope"] == "fixed"
	return fmt.Sprint(args["scope"]), nil
}

// TestSubagentBindings covers the standalone subagent runner, without parent delegation.
func TestSubagentBindings(t *testing.T) {
	probe := &boundProbe{fakeTool: fakeTool{name: "Probe"}}
	bound, err := (tool.Bindings{"Probe": {"scope": "fixed"}}).Bind(tool.Tools{probe})
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{responses: []message.Message{toolCallMsg("one", "Probe", map[string]any{"scope": "override"}), textMsg("done")}}
	r := newRunner(provider, bound)
	if _, err := r.Run(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	if !probe.executed || probe.pathsCallCount == 0 {
		t.Fatal("bound execution lost optional hooks/paths")
	}
	if _, visible := provider.requests[0].Tools[0].Parameters.Properties["scope"]; visible {
		t.Fatal("fixed input in model schema")
	}
}
