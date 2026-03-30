package override

import (
	"strings"
	"testing"
	"time"
)

// ── Test structs mirroring real Aura config types ───────────────────────

type Features struct {
	Tools      ToolExecution `yaml:"tools"`
	Compaction Compaction    `yaml:"compaction"`
	Guardrail  Guardrail     `yaml:"guardrail"`
	Sandbox    Sandbox       `yaml:"sandbox"`
	Vision     Vision        `yaml:"vision"`
	Subagent   Subagent      `yaml:"subagent"`
}

type ToolExecution struct {
	Mode             string         `yaml:"mode"`
	MaxSteps         int            `yaml:"max_steps"`
	TokenBudget      int            `yaml:"token_budget"`
	Bash             ToolBash       `yaml:"bash"`
	Parallel         *bool          `yaml:"parallel"`
	Enabled          []string       `yaml:"enabled"`
	Disabled         []string       `yaml:"disabled"`
	RejectionMessage string         `yaml:"rejection_message"`
	WebFetchMax      int64          `yaml:"webfetch_max_body_size"`
	ReadBefore       ReadBeforeConf `yaml:"read_before"`
}

type ToolBash struct {
	Rewrite    string         `yaml:"rewrite"`
	Truncation BashTruncation `yaml:"truncation"`
}

type BashTruncation struct {
	MaxLines  *int `yaml:"max_lines"`
	HeadLines *int `yaml:"head_lines"`
	TailLines *int `yaml:"tail_lines"`
}

type ReadBeforeConf struct {
	Write  *bool `yaml:"write"`
	Delete *bool `yaml:"delete"`
}

type Compaction struct {
	Threshold float64 `yaml:"threshold"`
	MaxTokens int     `yaml:"max_tokens"`
	Agent     string  `yaml:"agent"`
}

type Guardrail struct {
	Mode    string        `yaml:"mode"`
	OnError string        `yaml:"on_error"`
	Timeout time.Duration `yaml:"timeout"`
}

type Sandbox struct {
	Enabled *bool `yaml:"enabled"`
}

type Vision struct {
	Dimension int `yaml:"dimension"`
	Quality   int `yaml:"quality"`
}

type Subagent struct {
	MaxSteps int `yaml:"max_steps"`
}

type Generation struct {
	Temperature *float64 `yaml:"temperature"`
	TopK        *int     `yaml:"top_k"`
	ThinkBudget *int     `yaml:"think_budget"`
	Stop        []string `yaml:"stop"`
}

type Model struct {
	Name       string      `yaml:"name"`
	Provider   string      `yaml:"provider"`
	Context    int         `yaml:"context"`
	Generation *Generation `yaml:"generation"`
}

// OverrideTarget mirrors config.OverrideTarget — the unified struct for --override.
type OverrideTarget struct {
	Features Features `yaml:"features"`
	Model    Model    `yaml:"model"`
}

// ── Helpers ─────────────────────────────────────────────────────────────

func intPtr(v int) *int          { return &v }
func boolPtr(v bool) *bool       { return &v }
func floatPtr(v float64) *float64 { return &v }

func seed() OverrideTarget {
	return OverrideTarget{
		Features: Features{
			Tools: ToolExecution{
				Mode: "percentage", MaxSteps: 50, TokenBudget: 100000,
				Bash: ToolBash{
					Rewrite:    "original",
					Truncation: BashTruncation{MaxLines: intPtr(200), HeadLines: intPtr(100), TailLines: intPtr(80)},
				},
				Parallel: boolPtr(true),
				Disabled: []string{"Bash", "Write"},
			},
			Compaction: Compaction{Threshold: 0.8, MaxTokens: 10000, Agent: "Compaction"},
			Guardrail:  Guardrail{Mode: "log", Timeout: 2 * time.Minute},
			Vision:     Vision{Dimension: 1024, Quality: 75},
			Subagent:   Subagent{MaxSteps: 25},
		},
		Model: Model{Name: "llama3", Provider: "ollama", Context: 128000},
	}
}

// ── Tests ───────────────────────────────────────────────────────────────

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw      string
		wantPath string
		wantVal  string
		wantErr  string
	}{
		{"features.tools.max_steps=100", "features.tools.max_steps", "100", ""},
		{"model.name=claude", "model.name", "claude", ""},
		{"features.tools.bash.rewrite=", "features.tools.bash.rewrite", "", ""},
		{"x.y=a=b=c", "x.y", "a=b=c", ""},
		{"no-equals", "", "", "missing '='"},
		{"=100", "", "", "empty key"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()

			path, val, err := Parse(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error %q, got %v", tt.wantErr, err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if path != tt.wantPath {
				t.Fatalf("path: got %q, want %q", path, tt.wantPath)
			}

			if val != tt.wantVal {
				t.Fatalf("val: got %q, want %q", val, tt.wantVal)
			}
		})
	}
}

func TestDotToYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path, value, want string
	}{
		{"max_steps", "100", "max_steps: 100\n"},
		{"bash.truncation.max_lines", "300", "bash:\n  truncation:\n    max_lines: 300\n"},
		{"enabled", "[Read, Glob]", "enabled: [Read, Glob]\n"},
		{"mode", "", "mode: \"\"\n"},
		{"msg", "Error: too large (limit=%d)", "msg: \"Error: too large (limit=%d)\"\n"},
	}

	for _, tt := range tests {
		t.Run(tt.path+"="+tt.value, func(t *testing.T) {
			t.Parallel()

			got := DotToYAML(tt.path, tt.value)
			if got != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// ── Feature overrides ───────────────────────────────────────────────────

func TestFeature_Int(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.max_steps=100"))
	eq(t, cfg.Features.Tools.MaxSteps, 100)
}

func TestFeature_IntZero(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.max_steps=0"))
	eq(t, cfg.Features.Tools.MaxSteps, 0)
}

func TestFeature_Float(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.compaction.threshold=0.95"))
	eq(t, cfg.Features.Compaction.Threshold, 0.95)
}

func TestFeature_FloatZero(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.compaction.threshold=0"))
	eq(t, cfg.Features.Compaction.Threshold, 0.0)
}

func TestFeature_String(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.guardrail.mode=block"))
	eq(t, cfg.Features.Guardrail.Mode, "block")
}

func TestFeature_StringEmpty(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.guardrail.mode="))
	eq(t, cfg.Features.Guardrail.Mode, "")
}

func TestFeature_Int64(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.webfetch_max_body_size=10485760"))
	eq(t, cfg.Features.Tools.WebFetchMax, int64(10485760))
}

func TestFeature_PtrBoolTrue(t *testing.T) {
	t.Parallel()

	cfg := seed()
	cfg.Features.Sandbox.Enabled = nil
	must(t, Apply(&cfg, "features.sandbox.enabled=true"))
	ptrEq(t, cfg.Features.Sandbox.Enabled, true)
}

func TestFeature_PtrBoolFalse(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.sandbox.enabled=false"))
	ptrEq(t, cfg.Features.Sandbox.Enabled, false)
}

func TestFeature_PtrInt(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.bash.truncation.max_lines=300"))
	ptrEq(t, cfg.Features.Tools.Bash.Truncation.MaxLines, 300)
}

func TestFeature_PtrIntZero(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.bash.truncation.max_lines=0"))
	ptrEq(t, cfg.Features.Tools.Bash.Truncation.MaxLines, 0)
}

func TestFeature_Duration(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.guardrail.timeout=30s"))
	eq(t, cfg.Features.Guardrail.Timeout, 30*time.Second)
}

func TestFeature_DurationZero(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.guardrail.timeout=0"))
	eq(t, cfg.Features.Guardrail.Timeout, time.Duration(0))
}

func TestFeature_Slice(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.enabled=[Read, Glob, Grep]"))
	eq(t, len(cfg.Features.Tools.Enabled), 3)
	eq(t, cfg.Features.Tools.Enabled[0], "Read")
}

func TestFeature_SliceClear(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.disabled=[]"))
	eq(t, len(cfg.Features.Tools.Disabled), 0)
}

func TestFeature_ValueWithEquals(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.rejection_message=Error: too large (limit=%d)"))

	if !strings.Contains(cfg.Features.Tools.RejectionMessage, "limit=%d") {
		t.Fatalf("value with = not preserved: %q", cfg.Features.Tools.RejectionMessage)
	}
}

// ── Model overrides ─────────────────────────────────────────────────────

func TestModel_Context(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "model.context=200000"))
	eq(t, cfg.Model.Context, 200000)
}

func TestModel_Provider(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "model.provider=anthropic"))
	eq(t, cfg.Model.Provider, "anthropic")
}

func TestModel_Name(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "model.name=claude-sonnet-4-6"))
	eq(t, cfg.Model.Name, "claude-sonnet-4-6")
}

func TestModel_GenerationTemperature(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "model.generation.temperature=0.7"))

	if cfg.Model.Generation == nil {
		t.Fatal("Generation not allocated")
	}

	ptrEq(t, cfg.Model.Generation.Temperature, 0.7)
}

func TestModel_GenerationTemperatureZero(t *testing.T) {
	t.Parallel()

	cfg := seed()
	cfg.Model.Generation = &Generation{Temperature: floatPtr(0.9)}
	must(t, Apply(&cfg, "model.generation.temperature=0"))
	ptrEq(t, cfg.Model.Generation.Temperature, 0.0)
}

func TestModel_GenerationPreservesOther(t *testing.T) {
	t.Parallel()

	cfg := seed()
	cfg.Model.Generation = &Generation{Temperature: floatPtr(0.5), TopK: intPtr(40)}
	must(t, Apply(&cfg, "model.generation.temperature=0.9"))
	ptrEq(t, cfg.Model.Generation.Temperature, 0.9)
	ptrEq(t, cfg.Model.Generation.TopK, 40)
}

func TestModel_GenerationStop(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "model.generation.stop=[END, ---]"))
	eq(t, len(cfg.Model.Generation.Stop), 2)
	eq(t, cfg.Model.Generation.Stop[0], "END")
}

// ── Cross-cutting ───────────────────────────────────────────────────────

func TestPreservesOtherFields(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.max_steps=10"))
	eq(t, cfg.Features.Tools.Mode, "percentage")
	eq(t, cfg.Features.Tools.TokenBudget, 100000)
	eq(t, cfg.Features.Tools.Bash.Rewrite, "original")
	eq(t, cfg.Features.Compaction.Agent, "Compaction")
	eq(t, cfg.Features.Vision.Quality, 75)
	eq(t, cfg.Model.Name, "llama3")
	eq(t, cfg.Model.Context, 128000)
}

func TestDeepNestedPreservation(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, Apply(&cfg, "features.tools.bash.truncation.max_lines=999"))
	ptrEq(t, cfg.Features.Tools.Bash.Truncation.MaxLines, 999)
	ptrEq(t, cfg.Features.Tools.Bash.Truncation.HeadLines, 100)
	ptrEq(t, cfg.Features.Tools.Bash.Truncation.TailLines, 80)
	eq(t, cfg.Features.Tools.Bash.Rewrite, "original")
}

func TestApplyAll_MultipleLastWins(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, ApplyAll(&cfg, []string{
		"features.tools.max_steps=10",
		"features.guardrail.mode=block",
		"features.guardrail.timeout=30s",
		"model.context=200000",
		"model.generation.temperature=0.5",
		"features.tools.max_steps=99",
	}))
	eq(t, cfg.Features.Tools.MaxSteps, 99)
	eq(t, cfg.Features.Guardrail.Mode, "block")
	eq(t, cfg.Features.Guardrail.Timeout, 30*time.Second)
	eq(t, cfg.Model.Context, 200000)
	ptrEq(t, cfg.Model.Generation.Temperature, 0.5)
}

func TestApplyAll_FeaturesAndModelMixed(t *testing.T) {
	t.Parallel()

	cfg := seed()
	must(t, ApplyAll(&cfg, []string{
		"features.tools.max_steps=7",
		"model.name=gpt-4",
		"model.generation.temperature=0.3",
		"features.compaction.threshold=0.5",
	}))
	eq(t, cfg.Features.Tools.MaxSteps, 7)
	eq(t, cfg.Model.Name, "gpt-4")
	ptrEq(t, cfg.Model.Generation.Temperature, 0.3)
	eq(t, cfg.Features.Compaction.Threshold, 0.5)
	// Untouched fields preserved
	eq(t, cfg.Features.Tools.Mode, "percentage")
	eq(t, cfg.Model.Provider, "ollama")
	eq(t, cfg.Model.Context, 128000)
}

// ── Error cases ─────────────────────────────────────────────────────────

func TestError_UnknownFeatureField(t *testing.T) {
	t.Parallel()

	cfg := seed()
	err := Apply(&cfg, "features.tools.bogus=123")
	errContains(t, err, "not found")
}

func TestError_UnknownTopLevel(t *testing.T) {
	t.Parallel()

	cfg := seed()
	err := Apply(&cfg, "nonexistent.foo=bar")
	errContains(t, err, "not found")
}

func TestError_UnknownModelField(t *testing.T) {
	t.Parallel()

	cfg := seed()
	err := Apply(&cfg, "model.bogus=123")
	errContains(t, err, "not found")
}

func TestError_MissingEquals(t *testing.T) {
	t.Parallel()

	cfg := seed()
	err := Apply(&cfg, "features.tools.max_steps")
	errContains(t, err, "missing '='")
}

func TestError_EmptyKey(t *testing.T) {
	t.Parallel()

	cfg := seed()
	err := Apply(&cfg, "=100")
	errContains(t, err, "empty key")
}

// ── Test helpers ────────────────────────────────────────────────────────

func must(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()

	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func ptrEq[T comparable](t *testing.T, ptr *T, want T) {
	t.Helper()

	if ptr == nil {
		t.Fatalf("got nil, want &%v", want)
	}

	if *ptr != want {
		t.Fatalf("got &%v, want &%v", *ptr, want)
	}
}

func errContains(t *testing.T, err error, substr string) {
	t.Helper()

	if err == nil {
		t.Fatalf("want error containing %q, got nil", substr)
	}

	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("want error containing %q, got: %v", substr, err)
	}
}
