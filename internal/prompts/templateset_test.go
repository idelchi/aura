package prompts_test

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/prompts"
)

// Test data types matching the real TemplateData structure.

type testTemplateData struct {
	Agent   string
	Mode    testModeData
	Model   testModelData
	Tools   testToolsData
	Sandbox testSandboxData
	Vars    map[string]any
}

type testModeData struct {
	Name        string
	Description string
}

type testModelData struct {
	Name          string
	ParameterSize string
}

type testToolsData struct {
	Eager    []string
	Deferred string
}

type testSandboxData struct {
	Requested bool
	Display   string
}

// =============================================================================
// DAG cycle detection
// =============================================================================

func TestDAG_NoCycle(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System. {{ template "agent" . }} {{ template "mode" . }}`)
	ts.Register("agent", `Agent.`)
	ts.Register("mode", `Mode.`)

	if err := ts.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDAG_DirectCycle(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System. {{ template "agent" . }}`)
	ts.Register("agent", `Agent. {{ template "system" . }}`)

	err := ts.Validate()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}

	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention 'cycle': %v", err)
	}
}

func TestDAG_IndirectCycle(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System. {{ template "agent" . }}`)
	ts.Register("agent", `Agent. {{ template "mode" . }}`)
	ts.Register("mode", `Mode. {{ template "system" . }}`)

	if err := ts.Validate(); err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestDAG_SelfReference(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("agent")
	ts.Register("agent", `Agent. {{ template "agent" . }}`)

	if err := ts.Validate(); err == nil {
		t.Fatal("expected self-reference cycle error, got nil")
	}
}

func TestDAG_MissingDependency(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System. {{ template "agent" . }}`)

	if err := ts.Validate(); err == nil {
		t.Fatal("expected missing dependency error, got nil")
	}
}

func TestDAG_ConditionalReference_StillValidated(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `{{ if .HasMode }}{{ template "mode" . }}{{ end }}`)

	if err := ts.Validate(); err == nil {
		t.Fatal("expected missing dependency error even for conditional reference")
	}
}

func TestDAG_ConditionalReference_WithRegistered(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `{{ if .HasMode }}{{ template "mode" . }}{{ end }}`)
	ts.Register("mode", `Mode content.`)

	if err := ts.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDAG_UnusedPartial(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System. {{ template "agent" . }}`)
	ts.Register("agent", `Agent.`)
	ts.Register("mode", `Mode.`) // unused

	if err := ts.Validate(); err != nil {
		t.Fatalf("unused partials should not cause error: %v", err)
	}
}

func TestDAG_DiamondDependency(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `{{ template "agent" . }} {{ template "mode" . }}`)
	ts.Register("agent", `Agent. {{ template "shared" . }}`)
	ts.Register("mode", `Mode. {{ template "shared" . }}`)
	ts.Register("shared", `Shared content.`)

	if err := ts.Validate(); err != nil {
		t.Fatalf("diamond dependency should be valid: %v", err)
	}

	result, err := ts.Render(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if count := strings.Count(result, "Shared content."); count != 2 {
		t.Errorf("expected shared content to appear twice, got %d", count)
	}
}

// =============================================================================
// missingkey=error with struct-based TemplateData
// =============================================================================

func TestMissingKey_StructFieldExists_EmptyValue(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `Agent: {{ .Agent }}, Mode: {{ .Mode.Name }}`)

	data := testTemplateData{Agent: "high", Mode: testModeData{Name: ""}}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatalf("empty field should not error: %v", err)
	}

	if !strings.Contains(result, "Mode: ") {
		t.Error("empty mode name should render as empty string")
	}
}

func TestMissingKey_StructFieldDoesNotExist(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `Agent: {{ .Agent }}, Bogus: {{ .Bogus }}`)

	data := testTemplateData{Agent: "high"}

	_, err := ts.Render(data)
	if err == nil {
		t.Fatal("expected error for non-existent struct field")
	}
}

func TestMissingKey_NestedStructFieldDoesNotExist(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `Mode: {{ .Mode.Bogus }}`)

	data := testTemplateData{Mode: testModeData{Name: "edit"}}

	_, err := ts.Render(data)
	if err == nil {
		t.Fatal("expected error for non-existent nested field")
	}
}

// =============================================================================
// Full composition scenarios
// =============================================================================

func TestComposition_SystemAsCompositor_FullFeatures(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `You are a coding assistant using {{ .Model.Name }} ({{ .Model.ParameterSize }}).

{{ template "agent" . }}

{{ if .Mode.Name }}{{ template "mode" . }}{{ end }}

{{- if .Sandbox.Requested }}

## Restrictions
{{ .Sandbox.Display }}
{{- end }}

Available tools: {{ range $i, $t := .Tools.Eager }}{{ if $i }}, {{ end }}{{ $t }}{{ end }}`)

	ts.Register("agent", `## Agent: {{ .Agent }}
Follow user instructions carefully. Be concise.`)

	ts.Register("mode", `## Active Mode: {{ .Mode.Name }}
{{ .Mode.Description }}`)

	data := testTemplateData{
		Agent:   "high",
		Mode:    testModeData{Name: "edit", Description: "Only edit existing files, never create new ones."},
		Model:   testModelData{Name: "claude-opus-4", ParameterSize: "?"},
		Tools:   testToolsData{Eager: []string{"Read", "Write", "Patch", "Glob", "Bash"}},
		Sandbox: testSandboxData{Requested: true, Display: "ReadOnly: /home\nReadWrite: /project"},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"claude-opus-4",
		"## Agent: high",
		"## Active Mode: edit",
		"Only edit existing files",
		"## Restrictions",
		"ReadOnly: /home",
		"Read, Write, Patch, Glob, Bash",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("missing expected content: %q", want)
		}
	}
}

func TestComposition_NoSystem_AgentIsCompositor(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("agent")
	ts.Register("agent", `I am a minimal agent.

{{ if .Mode.Name }}{{ template "mode" . }}{{ end }}

Tools: {{ range $i, $t := .Tools.Eager }}{{ if $i }}, {{ end }}{{ $t }}{{ end }}`)

	ts.Register("mode", `## {{ .Mode.Name }}
{{ .Mode.Description }}`)

	data := testTemplateData{
		Agent: "minimal",
		Mode:  testModeData{Name: "ask", Description: "Read-only mode. No file modifications."},
		Tools: testToolsData{Eager: []string{"Read", "Glob", "Grep"}},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## ask") {
		t.Error("mode should be rendered")
	}

	if !strings.Contains(result, "Read, Glob, Grep") {
		t.Error("tools should be rendered")
	}
}

func TestComposition_NoMode(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ template "agent" . }}
{{ if .Mode.Name }}{{ template "mode" . }}{{ end }}`)
	ts.Register("agent", `Agent content.`)
	ts.Register("mode", `Mode: {{ .Mode.Name }}`)

	data := testTemplateData{Agent: "high", Mode: testModeData{}}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "Mode:") {
		t.Error("mode section should not appear")
	}
}

func TestComposition_ModeReorderedBeforeAgent(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ if .Mode.Name }}{{ template "mode" . }}{{ end }}
{{ template "agent" . }}`)
	ts.Register("agent", `Agent content.`)
	ts.Register("mode", `## Mode: {{ .Mode.Name }}`)

	data := testTemplateData{Agent: "high", Mode: testModeData{Name: "edit"}}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	modeIdx := strings.Index(result, "## Mode:")
	agentIdx := strings.Index(result, "Agent content.")

	if modeIdx >= agentIdx {
		t.Error("mode should appear BEFORE agent in reordered layout")
	}
}

func TestComposition_WorkspaceInstructions(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ template "agent" . }}
{{ template "workspace" . }}`)
	ts.Register("agent", `Agent.`)
	ts.Register("workspace", `## Workspace Instructions
Follow the project conventions in CLAUDE.md.`)

	result, err := ts.Render(testTemplateData{Agent: "high"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## Workspace Instructions") {
		t.Error("workspace should be rendered")
	}
}

func TestComposition_AutoloadedFiles(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ template "agent" . }}
{{ template "files" . }}`)
	ts.Register("agent", `Agent.`)
	ts.Register("files", `## File 1
Content of file 1.

## File 2
Content of file 2 with model {{ .Model.Name }}.`)

	data := testTemplateData{Agent: "high", Model: testModelData{Name: "gpt-4"}}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Content of file 2 with model gpt-4") {
		t.Error("files should render with template data")
	}
}

func TestComposition_ModeIncludesAgent(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `{{ template "mode" . }}`)
	ts.Register("mode", `Mode. {{ template "agent" . }}`)
	ts.Register("agent", `Agent.`)

	if err := ts.Validate(); err != nil {
		t.Fatalf("mode→agent (no cycle) should be valid: %v", err)
	}

	result, err := ts.Render(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Mode. Agent.") {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestComposition_SprigFunctions(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `Today: {{ now | date "2006-01-02" }}. {{ template "agent" . }}`)
	ts.Register("agent", `Tools: {{ join ", " .Tools }}`)

	data := map[string]any{
		"Tools": []string{"Read", "Write", "Bash"},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Read, Write, Bash") {
		t.Error("sprig join should work")
	}
}

func TestComposition_EntryPointNotRegistered(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("agent", `Agent.`)

	_, err := ts.Render(map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing entry point")
	}
}

func TestComposition_EmptyModeBody(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System. {{ template "mode" . }}`)
	ts.Register("mode", ``)

	result, err := ts.Render(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if result != "System. " {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestComposition_ModeAsEntryPoint(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("mode")
	ts.Register("mode", `Mode controls all. {{ template "agent" . }}`)
	ts.Register("agent", `Agent: {{ .Agent }}`)

	data := testTemplateData{Agent: "custom"}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Agent: custom") {
		t.Error("agent should render inside mode")
	}
}

// =============================================================================
// Optional components
// =============================================================================

func TestComposition_AlwaysRegisterOptionals_NoMode(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ template "agent" . }}
{{ if .Mode.Name }}{{ template "mode" . }}{{ end }}`)
	ts.Register("agent", `Agent.`)
	ts.Register("mode", ``)

	data := testTemplateData{Mode: testModeData{Name: ""}}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatalf("empty optionals should work: %v", err)
	}

	if strings.Contains(result, "Mode") {
		t.Error("mode should not appear")
	}
}

func TestComposition_AlwaysRegisterOptionals_AllActive(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ template "agent" . }}
{{ if .Mode.Name }}{{ template "mode" . }}{{ end }}`)
	ts.Register("agent", `Agent: {{ .Agent }}.`)
	ts.Register("mode", `## Mode: {{ .Mode.Name }}`)

	data := testTemplateData{
		Agent: "high",
		Mode:  testModeData{Name: "edit"},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"Agent: high", "## Mode: edit"} {
		if !strings.Contains(result, want) {
			t.Errorf("missing: %q", want)
		}
	}
}

// =============================================================================
// Inheritance simulation
// =============================================================================

func TestComposition_InheritedSystemPrompt(t *testing.T) {
	t.Parallel()

	parent := `You are a coding assistant.
{{ template "agent" . }}`
	child := `## Project Rules
Always use gofmt.`

	concatenated := parent + "\n\n" + child

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", concatenated)
	ts.Register("agent", `Agent: {{ .Agent }}`)
	ts.Register("mode", ``)

	data := map[string]any{"Agent": "high"}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Agent: high") {
		t.Error("agent should be included via parent's template call")
	}

	if !strings.Contains(result, "Always use gofmt") {
		t.Error("child instructions should appear")
	}

	if count := strings.Count(result, "Agent: high"); count != 1 {
		t.Errorf("agent should appear exactly once, got %d", count)
	}
}

func TestComposition_InheritedSystemPrompt_ChildOverridesLayout(t *testing.T) {
	t.Parallel()

	parent := `You are a coding assistant.`
	child := `{{ if .Mode.Name }}{{ template "mode" . }}{{ end }}
{{ template "agent" . }}`

	concatenated := parent + "\n\n" + child

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", concatenated)
	ts.Register("agent", `Agent instructions.`)
	ts.Register("mode", `## Mode: {{ .Mode.Name }}`)

	data := map[string]any{
		"Mode": map[string]any{"Name": "edit"},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	modeIdx := strings.Index(result, "## Mode:")
	agentIdx := strings.Index(result, "Agent instructions.")

	if modeIdx >= agentIdx {
		t.Error("mode should appear BEFORE agent (child controls layout)")
	}
}

// =============================================================================
// Parse errors
// =============================================================================

func TestComposition_ParseError(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System. {{ template "agent" . }}`)
	ts.Register("agent", `Agent. {{ .Broken `)

	err := ts.Validate()
	if err == nil {
		t.Fatal("expected parse error")
	}

	if !strings.Contains(err.Error(), "parsing template") {
		t.Errorf("error should mention parsing: %v", err)
	}
}

// =============================================================================
// Range in named templates
// =============================================================================

func TestComposition_RangeInNamedTemplate(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System. {{ template "tools" . }}`)
	ts.Register("tools", `Available tools:
{{ range .Tools }}- {{ . }}
{{ end }}`)

	data := map[string]any{
		"Tools": []string{"Read", "Write", "Bash", "Glob"},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range []string{"- Read", "- Write", "- Bash", "- Glob"} {
		if !strings.Contains(result, tool) {
			t.Errorf("missing tool: %q", tool)
		}
	}
}

// =============================================================================
// Deep nesting
// =============================================================================

func TestComposition_DeeplyNested(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `L0: {{ template "agent" . }}`)
	ts.Register("agent", `L1: {{ template "mode" . }}`)
	ts.Register("mode", `L2: {{ template "tools" . }}`)
	ts.Register("tools", `L3: {{ range .Tools }}{{ . }} {{ end }}`)

	data := map[string]any{"Tools": []string{"Read", "Write"}}

	if err := ts.Validate(); err != nil {
		t.Fatalf("deep nesting should be valid: %v", err)
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "L0: L1: L2: L3: Read Write") {
		t.Errorf("all levels should render: %q", result)
	}
}

// =============================================================================
// Iterable components (dynamic include)
// =============================================================================

type fileEntry struct {
	Name         string
	TemplateName string
}

func TestIterable_FilesAsRange(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ template "agent" . }}

## Autoloaded Files
{{ range .Files }}### {{ .Name }}
{{ include .TemplateName $ }}
{{ end }}`)
	ts.Register("agent", `Agent: {{ .Agent }}`)
	ts.Register("file:api", `API reference for {{ .Model.Name }}.`)
	ts.Register("file:guidelines", `Always use {{ .Model.Name }} best practices.`)

	data := map[string]any{
		"Agent": "high",
		"Model": map[string]any{"Name": "claude-opus-4"},
		"Files": []fileEntry{
			{Name: "api-reference.md", TemplateName: "file:api"},
			{Name: "coding-guidelines.md", TemplateName: "file:guidelines"},
		},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"Agent: high",
		"### api-reference.md",
		"API reference for claude-opus-4",
		"### coding-guidelines.md",
		"Always use claude-opus-4 best practices",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("missing: %q", want)
		}
	}
}

func TestIterable_WorkspaceAsRange(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ range .Workspace }}## Workspace: {{ .Type }}
{{ include .TemplateName $ }}
{{ end }}`)
	ts.Register("ws:agents", `Project agents rules here.`)
	ts.Register("ws:claude", `Claude-specific instructions. Model: {{ .Model.Name }}`)

	data := map[string]any{
		"Model": map[string]any{"Name": "gpt-4"},
		"Workspace": []map[string]any{
			{"Type": "AGENTS.md", "TemplateName": "ws:agents"},
			{"Type": "CLAUDE.md", "TemplateName": "ws:claude"},
		},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"## Workspace: AGENTS.md",
		"Project agents rules here",
		"## Workspace: CLAUDE.md",
		"Claude-specific instructions. Model: gpt-4",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("missing: %q", want)
		}
	}
}

func TestIterable_EmptyFiles(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `System.
{{ range .Files }}{{ include .TemplateName $ }}{{ end }}Done.`)

	data := map[string]any{
		"Files": []fileEntry{},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "System.\nDone.") {
		t.Errorf("empty range should produce no output: %q", result)
	}
}

func TestIterable_IncludeDetectedByDAG(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `{{ include "agent" . }}`)
	ts.Register("agent", `{{ include "system" . }}`) // cycle via include

	if err := ts.Validate(); err == nil {
		t.Fatal("expected cycle error via include")
	}
}

func TestIterable_IncludeMissingTemplate(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `{{ include "nonexistent" . }}`)

	if err := ts.Validate(); err == nil {
		t.Fatal("expected missing dependency error via include")
	}
}

func TestIterable_DynamicIncludeNotCaughtByDAG(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `{{ range .Files }}{{ include .TemplateName $ }}{{ end }}`)

	data := map[string]any{
		"Files": []fileEntry{
			{Name: "test", TemplateName: "file:test"},
		},
	}

	// Validate passes — DAG can't see dynamic names.
	if err := ts.Validate(); err != nil {
		t.Fatalf("DAG should pass (can't see dynamic names): %v", err)
	}

	// Render fails — template doesn't exist.
	_, err := ts.Render(data)
	if err == nil {
		t.Fatal("expected render error for missing dynamic template")
	}
}

func TestIterable_MixedStaticAndDynamic(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("system")
	ts.Register("system", `{{ template "agent" . }}
{{ range .Files }}{{ include .TemplateName $ }}
{{ end }}`)
	ts.Register("agent", `Agent.`)
	ts.Register("file:readme", `README content for {{ .Model.Name }}.`)

	data := map[string]any{
		"Model": map[string]any{"Name": "claude"},
		"Files": []fileEntry{
			{Name: "README", TemplateName: "file:readme"},
		},
	}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Agent.") {
		t.Error("static template should render")
	}

	if !strings.Contains(result, "README content for claude") {
		t.Error("dynamic include should render")
	}
}

// =============================================================================
// missingkey=error with map access (Vars safety patterns)
// =============================================================================

func TestMissingKeyError_MapDirect_Errors(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("t")
	ts.Register("t", `Value: {{ .Vars.missing }}`)

	data := map[string]any{"Vars": map[string]any{"present": "yes"}}

	_, err := ts.Render(data)
	if err == nil {
		t.Fatal("expected error for missing map key with missingkey=error")
	}
}

func TestMissingKeyError_Index_NoError(t *testing.T) {
	t.Parallel()

	// index doesn't error for missing keys (unlike direct .Vars.missing access).
	// With map[string]any, missing keys return nil which renders as "<no value>".
	// Use {{ index .Vars "key" | default "" }} for empty string on missing.
	ts := prompts.NewTemplateSet("t")
	ts.Register("t", `Value: [{{ index .Vars "missing" }}]`)

	data := map[string]any{"Vars": map[string]any{"present": "yes"}}

	_, err := ts.Render(data)
	if err != nil {
		t.Fatalf("index should not error: %v", err)
	}
}

func TestMissingKeyError_Index_DefaultSafe(t *testing.T) {
	t.Parallel()

	// The recommended safe pattern: index + default produces empty string.
	ts := prompts.NewTemplateSet("t")
	ts.Register("t", `Value: [{{ index .Vars "missing" | default "" }}]`)

	data := map[string]any{"Vars": map[string]any{"present": "yes"}}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatalf("index+default should be safe: %v", err)
	}

	if !strings.Contains(result, "Value: []") {
		t.Errorf("index+default should produce empty string, got: %q", result)
	}
}

func TestMissingKeyError_Index_WithPresent(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("t")
	ts.Register("t", `Project: {{ index .Vars "project" }}`)

	data := map[string]any{"Vars": map[string]any{"project": "aura"}}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Project: aura") {
		t.Error("present key should render value")
	}
}

func TestMissingKeyError_StructFieldsUnaffected(t *testing.T) {
	t.Parallel()

	ts := prompts.NewTemplateSet("t")
	ts.Register("t", `Agent: {{ .Agent }}, Mode: {{ .Mode.Name }}`)

	data := testTemplateData{Agent: "high", Mode: testModeData{}, Vars: map[string]any{}}

	result, err := ts.Render(data)
	if err != nil {
		t.Fatalf("struct fields should never error: %v", err)
	}

	if !strings.Contains(result, "Agent: high, Mode:") {
		t.Errorf("unexpected result: %q", result)
	}
}
