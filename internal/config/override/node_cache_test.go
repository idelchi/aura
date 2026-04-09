package override

import (
	"strings"
	"testing"
	"time"
)

// Test that cached yaml.Node approach works with OverrideTarget-shaped structs,
// including zero values, preservation, and re-application across turns.

type cacheFeatures struct {
	Tools      cacheTools      `yaml:"tools"`
	Compaction cacheCompaction `yaml:"compaction"`
	Guardrail  cacheGuardrail  `yaml:"guardrail"`
}

type cacheTools struct {
	MaxSteps    int    `yaml:"max_steps"`
	TokenBudget int    `yaml:"token_budget"`
	Mode        string `yaml:"mode"`
}

type cacheCompaction struct {
	Threshold float64 `yaml:"threshold"`
	Agent     string  `yaml:"agent"`
}

type cacheGuardrail struct {
	Mode    string        `yaml:"mode"`
	Timeout time.Duration `yaml:"timeout"`
}

type cacheGeneration struct {
	Temperature *float64 `yaml:"temperature"`
	TopK        *int     `yaml:"top_k"`
}

type cacheThinkValue struct {
	Value any
}

func (t *cacheThinkValue) UnmarshalYAML(unmarshal func(any) error) error {
	var b bool
	if err := unmarshal(&b); err == nil {
		t.Value = b
		return nil
	}

	var s string
	if err := unmarshal(&s); err == nil {
		t.Value = s
		return nil
	}

	return nil
}

type cacheModel struct {
	Name       string           `yaml:"name"`
	Provider   string           `yaml:"provider"`
	Think      cacheThinkValue  `yaml:"think"`
	Context    int              `yaml:"context"`
	Generation *cacheGeneration `yaml:"generation"`
}

type cacheTarget struct {
	Features cacheFeatures `yaml:"features"`
	Model    cacheModel    `yaml:"model"`
}

func cacheNodes(t *testing.T, overrides []string) []cachedNode {
	t.Helper()

	nodes := make([]cachedNode, len(overrides))

	for i, raw := range overrides {
		path, value, err := Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}

		yamlStr := DotToYAML(path, value)
		nodes[i] = cachedNode{yaml: yamlStr, raw: raw}
	}

	return nodes
}

type cachedNode struct {
	yaml string
	raw  string
}

func applyNodes(t *testing.T, target any, nodes []cachedNode) {
	t.Helper()

	for _, n := range nodes {
		if err := Apply(target, n.raw); err != nil {
			t.Fatalf("apply %q: %v", n.raw, err)
		}
	}
}

func TestCachedNodeApproach(t *testing.T) {
	t.Parallel()

	overrides := []string{
		"features.tools.max_steps=0",
		"features.compaction.threshold=0.5",
		"features.guardrail.timeout=30s",
		"model.name=gpt-4",
		"model.think=high",
		"model.context=200000",
		"model.generation.temperature=0.1",
	}

	// Validate: apply to empty target
	var probe cacheTarget
	must(t, ApplyAll(&probe, overrides))

	eq(t, probe.Features.Tools.MaxSteps, 0)
	eq(t, probe.Features.Compaction.Threshold, 0.5)
	eq(t, probe.Features.Guardrail.Timeout, 30*time.Second)
	eq(t, probe.Model.Name, "gpt-4")
	eq(t, probe.Model.Context, 200000)

	s, ok := probe.Model.Think.Value.(string)
	if !ok {
		t.Fatalf("think: expected string, got %T", probe.Model.Think.Value)
	}

	eq(t, s, "high")

	if probe.Model.Generation == nil || probe.Model.Generation.Temperature == nil {
		t.Fatal("generation.temperature not set")
	}

	ptrEq(t, probe.Model.Generation.Temperature, 0.1)

	// Simulate rebuildState turn 1: apply to scratch with real features
	effective := cacheFeatures{
		Tools:      cacheTools{MaxSteps: 300, Mode: "percentage"},
		Compaction: cacheCompaction{Threshold: 0.8, Agent: "Compaction"},
	}

	scratch := cacheTarget{Features: effective, Model: cacheModel{}}
	must(t, ApplyAll(&scratch, overrides))

	eq(t, scratch.Features.Tools.MaxSteps, 0)            // overridden to zero
	eq(t, scratch.Features.Tools.Mode, "percentage")      // preserved
	eq(t, scratch.Features.Compaction.Threshold, 0.5)     // overridden
	eq(t, scratch.Features.Compaction.Agent, "Compaction") // preserved

	// Simulate rebuildState turn 2: different baseline, overrides re-applied
	effective2 := cacheFeatures{
		Tools:      cacheTools{MaxSteps: 50, TokenBudget: 100000, Mode: "tokens"},
		Compaction: cacheCompaction{Threshold: 0.9, Agent: "NewAgent"},
	}

	scratch2 := cacheTarget{Features: effective2, Model: cacheModel{}}
	must(t, ApplyAll(&scratch2, overrides))

	eq(t, scratch2.Features.Tools.MaxSteps, 0)            // still overridden to zero
	eq(t, scratch2.Features.Tools.TokenBudget, 100000)    // preserved
	eq(t, scratch2.Features.Tools.Mode, "tokens")          // preserved (different from turn 1)
	eq(t, scratch2.Features.Compaction.Threshold, 0.5)     // still overridden
	eq(t, scratch2.Features.Compaction.Agent, "NewAgent")  // preserved (different from turn 1)
}

func TestCachedNodeUnknownField(t *testing.T) {
	t.Parallel()

	err := Apply(&cacheTarget{}, "features.tools.bogus=1")

	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCachedNodeUnknownSection(t *testing.T) {
	t.Parallel()

	err := Apply(&cacheTarget{}, "bogus.thing=1")

	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMaxStepsAsOverrideString(t *testing.T) {
	t.Parallel()

	// Simulates converting --max-steps=7 to an override string
	effective := cacheFeatures{Tools: cacheTools{MaxSteps: 300}}
	scratch := cacheTarget{Features: effective}
	must(t, Apply(&scratch, "features.tools.max_steps=7"))
	eq(t, scratch.Features.Tools.MaxSteps, 7)
}

func TestMaxStepsZeroAsOverrideString(t *testing.T) {
	t.Parallel()

	effective := cacheFeatures{Tools: cacheTools{MaxSteps: 300}}
	scratch := cacheTarget{Features: effective}
	must(t, Apply(&scratch, "features.tools.max_steps=0"))
	eq(t, scratch.Features.Tools.MaxSteps, 0)
}
