package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/idelchi/aura/internal/condition"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/internal/prompts"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/sandbox"
	"github.com/idelchi/aura/pkg/tokens"
	"github.com/idelchi/aura/pkg/wildcard"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Config holds loaded configuration from YAML files. Immutable after config.New() returns.
// Path context lives in Paths; CLI flags and computed state live in Runtime.
type Config struct {
	Providers StringCollection[Provider]
	// Systems contains all loaded system prompts.
	Systems Collection[System]
	// Modes contains all loaded operational modes.
	Modes Collection[Mode]
	// Agents contains all loaded agent configurations.
	Agents Collection[Agent]
	// AgentsMd contains all loaded AGENTS.md workspace instructions.
	AgentsMd AgentsMds
	// MCPs contains all loaded MCP server configurations.
	MCPs StringCollection[mcp.Server]
	// Features holds feature-level configuration (compaction, title, thinking).
	Features Features
	// Commands contains all loaded custom slash commands.
	Commands Collection[CustomCommand]
	// Skills contains all loaded LLM-invocable skill definitions.
	Skills Collection[Skill]
	// ToolDefs holds unified tool definitions loaded from .aura/config/tools/**/*.yaml.
	// Entries are text-only overrides for tool descriptions, usage, examples, and metadata (disabled, condition,
	// parallel).
	ToolDefs ToolDefs
	// Hooks holds user-configurable shell hooks loaded from .aura/config/hooks/**/*.yaml.
	Hooks Hooks
	// LSPServers holds LSP server configurations loaded from .aura/config/lsp/**/*.yaml.
	LSPServers LSPServers
	// Tasks holds scheduled task definitions loaded from .aura/config/tasks/**/*.yaml.
	Tasks task.Tasks
	// Plugins holds plugin definitions loaded from .aura/plugins/*/plugin.yaml.
	Plugins StringCollection[Plugin]
	// ApprovalRules holds persisted bash approval patterns from config/rules/**/*.yaml.
	ApprovalRules ApprovalRules
}

// Paths holds filesystem context for config resolution and tool execution.
// Immutable after construction (the assistant uses a taskWorkDir override separately).
type Paths struct {
	// Home is the config directory path (e.g. ".aura" or "/home/user/.aura").
	Home string
	// Global is the global config directory (~/.aura).
	Global string
	// Launch is the CWD at process start, before os.Chdir(--workdir).
	Launch string
	// Work is the effective working directory after os.Chdir(--workdir).
	Work string
}

// Runtime holds CLI flags and computed state. Mutable throughout the session.
// CLI flag fields are set once at startup and preserved across config reloads.
// Computed fields are set/updated by the assistant layer (rebuildState).
type Runtime struct {
	// CLI flags (set once at startup, preserved across reloads).

	// WithPlugins indicates whether user-defined Go plugins are enabled.
	WithPlugins bool
	// UnsafePlugins enables os/exec access for plugins (CLI --unsafe-plugins).
	UnsafePlugins bool
	// DisplayProviders holds CLI --providers filter for model listings.
	DisplayProviders []string
	// IncludeTools holds CLI --include-tools glob patterns.
	IncludeTools []string
	// ExcludeTools holds CLI --exclude-tools glob patterns.
	ExcludeTools []string
	// IncludeMCPs holds CLI --include-mcps glob patterns for MCP server filtering.
	IncludeMCPs []string
	// ExcludeMCPs holds CLI --exclude-mcps glob patterns for MCP server filtering.
	ExcludeMCPs []string

	// Computed state (set/updated by the assistant layer).

	// AllTools is the complete tool set including runtime-dependent tools (todo, vision, query).
	AllTools tool.Tools
	// ExtraFilter holds an additional runtime tool filter (e.g. task-level overrides).
	ExtraFilter *tool.Filter
	// OptInTools lists tool names that are hidden unless explicitly enabled by name or narrow glob.
	OptInTools []string
	// LoadedTools tracks tools loaded via LoadTools this session. Deferred tools
	// in this set stay eager on subsequent rebuilds.
	LoadedTools map[string]bool
	// ConditionState holds the current runtime state for evaluating tool/MCP conditions.
	// Nil means "skip all condition checks" (e.g. `aura tools` CLI, startup before model resolution).
	ConditionState *condition.State
	// Estimator is the session's token estimator. In a session, the assistant wires
	// UseNative with the current provider. Tools read this for token counting.
	// Nil in singletool CLI paths where no estimator is needed.
	Estimator *tokens.Estimator
}

// GlobalFilter returns the resolved global include/exclude patterns.
// CLI flags (rt.IncludeTools/ExcludeTools) take precedence; config
// (Features.ToolExecution.Enabled/Disabled) is the fallback.
// When rt is nil, only config-level patterns are returned.
func (c Config) GlobalFilter(rt *Runtime) (include, exclude []string) {
	if rt != nil {
		include = rt.IncludeTools
		exclude = rt.ExcludeTools
	}

	if len(include) == 0 {
		include = c.Features.ToolExecution.Enabled
	}

	if len(exclude) == 0 {
		exclude = c.Features.ToolExecution.Disabled
	}

	return include, exclude
}

// MCPFilter returns the resolved global include/exclude patterns for MCP servers.
// CLI flags (rt.IncludeMCPs/ExcludeMCPs) take precedence; config
// (Features.MCP.Enabled/Disabled) is the fallback.
// When rt is nil, only config-level patterns are returned.
func (c Config) MCPFilter(rt *Runtime) (include, exclude []string) {
	if rt != nil {
		include = rt.IncludeMCPs
		exclude = rt.ExcludeMCPs
	}

	if len(include) == 0 {
		include = c.Features.MCP.Enabled
	}

	if len(exclude) == 0 {
		exclude = c.Features.MCP.Disabled
	}

	return include, exclude
}

// ResolveFeatures computes the effective features for a specific agent+mode by merging
// global features with agent and mode overrides. Same merge chain as rebuildState().
func (c *Config) ResolveFeatures(globalFeatures Features, agentName, modeName string) (Features, error) {
	effective := globalFeatures

	if ag := c.Agents.Get(agentName); ag != nil {
		if err := effective.MergeFrom(ag.Metadata.Features); err != nil {
			return Features{}, fmt.Errorf("merging agent features: %w", err)
		}
	}

	if modeName != "" {
		if md := c.Modes.Get(modeName); md != nil {
			if err := effective.MergeFrom(md.Metadata.Features); err != nil {
				return Features{}, fmt.Errorf("merging mode features: %w", err)
			}
		}
	}

	return effective, nil
}

// EffectiveRestrictions returns the merged sandbox restrictions from the already-merged features.
// All layers (global, agent, mode, task) are merged by rebuildState() before this is called.
func (c Config) EffectiveRestrictions() Restrictions {
	return c.Features.Sandbox.EffectiveRestrictions()
}

// EffectiveToolPolicy returns the merged tool policy for a given agent and mode.
// Combines agent-level and mode-level tool policies additively (union for auto, confirm, and deny),
// then merges persisted approval rules from config/rules/**/*.yaml.
func (c Config) EffectiveToolPolicy(agent, mode string) ToolPolicy {
	// Start from global features.tools.policy as baseline.
	p := c.Features.ToolExecution.Policy

	if ag := c.Agents.Get(agent); ag != nil {
		p = p.Merge(ag.Metadata.Tools.Policy)
	}

	if md := c.Modes.Get(mode); md != nil {
		p = p.Merge(md.Metadata.Tools.Policy)
	}

	// Merge persisted approval rules into Auto (approval rules override confirm, never deny).
	p = p.Merge(ToolPolicy{Auto: c.ApprovalRules.Project.ToolAuto})
	p = p.Merge(ToolPolicy{Auto: c.ApprovalRules.Global.ToolAuto})

	// Store provenance for display.
	p.approvalsProject = c.ApprovalRules.Project.ToolAuto
	p.approvalsGlobal = c.ApprovalRules.Global.ToolAuto

	return p
}

// ReadAutoload reads a file path declared in agent frontmatter.
// Relative paths are resolved against the config home directory.
func (c Config) ReadAutoload(path string, paths Paths) (string, error) {
	f := file.New(path)
	if !f.IsAbs() {
		f = folder.New(paths.Home).WithFile(path)
	}

	content, err := f.ReadString()
	if err != nil {
		return "", fmt.Errorf("autoload file %q: %w", path, err)
	}

	return content, nil
}

// defaultCompositor is registered as the "system" template when an agent has no
// explicit system prompt (system: ""). It provides the standard composition
// layout: agent body, then files, workspace, and mode.
const defaultCompositor = `{{ template "agent" . }}
{{ range .Files }}
### {{ .Name }}
{{ include .TemplateName $ }}
{{ end }}
{{- range .Workspace }}

## {{ .Type }}
{{ include .TemplateName $ }}
{{ end }}
{{- if .Mode.Name }}

## Active Mode: {{ .Mode.Name }}
{{ template "mode" . }}
{{ end }}`

// BuildAgent assembles the prompt and tools for an agent with the specified mode and system prompt.
// Empty mode means no mode (no mode prompt or mode-specific tools).
// Empty system means use the agent's configured system prompt (or the default compositor if none).
// Callers are responsible for resolving defaults before calling this.
//
// The data parameter provides template context (model metadata, agent/mode names)
// to all prompt templates. The system prompt acts as a compositor that controls
// the inclusion order of agent, files, workspace, and mode components via
// {{ template "X" . }} and {{ include .TemplateName $ }} directives.
func (c Config) BuildAgent(
	agent, mode, system string,
	data TemplateData,
	paths Paths,
	rt *Runtime,
) (prompts.Prompt, Model, tool.Tools, tool.Tools, error) {
	// Agent lookup
	ag := c.Agents.Get(agent)
	if ag == nil {
		return "", Model{}, nil, nil, fmt.Errorf("agent %q not found", agent)
	}

	// Resolve tools early so {{ .Tools.Eager }} is available to all templates.
	eager, deferred, err := c.Tools(agent, mode, rt)
	if err != nil {
		return "", Model{}, nil, nil, err
	}

	data.Tools = ToolsData{Eager: eager.Names()}

	// Build deferred tool index for system prompt rendering.
	if len(deferred) > 0 {
		data.Tools.Deferred = NewDeferredToolIndex(deferred).Render()
	}

	// Sandbox data — BuildAgent uses config-level values (no runtime toggle here).
	// When called from the assistant layer, TemplateData() overwrites these with runtime state.
	sandboxEnabled := c.Features.Sandbox.IsEnabled()
	restrictions := c.EffectiveRestrictions()
	sandboxDisplay := ""

	if sandboxEnabled {
		sb := &sandbox.Sandbox{WorkDir: paths.Work}
		sb.AddReadOnly(restrictions.ReadOnly...)
		sb.AddReadWrite(restrictions.ReadWrite...)

		sandboxDisplay = sb.String(true)
	}

	data.Sandbox = NewSandboxData(sandboxEnabled, sandboxEnabled, restrictions, sandboxDisplay)

	// ReadBefore and ToolPolicy.
	rbPolicy := c.Features.ToolExecution.ReadBefore.ToPolicy()

	data.ReadBefore = ReadBeforeData{
		Write:  rbPolicy.Write,
		Delete: rbPolicy.Delete,
	}

	toolPolicy := c.EffectiveToolPolicy(agent, mode)

	data.ToolPolicy = ToolPolicyData{
		Auto:    toolPolicy.Auto,
		Confirm: toolPolicy.Confirm,
		Deny:    toolPolicy.Deny,
		Display: toolPolicy.Display(),
	}

	// Hooks data — filtered by agent/mode, same as runtime hooks compilation.
	data.Hooks = NewHooksData(c.FilteredHooks(agent, mode))

	// Populate path template variables for all template rendering.
	data.Config = ConfigPaths{
		Global:  paths.Global,
		Project: paths.Home,
		Source:  ag.Metadata.SourceHome,
	}
	data.LaunchDir = paths.Launch
	data.WorkDir = paths.Work

	memories, err := NewMemoriesData(paths.Home, paths.Global)
	if err != nil {
		return "", Model{}, nil, nil, fmt.Errorf("loading memories: %w", err)
	}

	data.Memories = memories

	// ── Template Composition ────────────────────────────────────────────
	// All prompt components are registered as named templates in a TemplateSet.
	// The system prompt (or a default compositor) is the entry point that
	// controls the inclusion order of all components.

	ts := prompts.NewTemplateSet("system")

	// Register system prompt or default compositor.
	// The system parameter takes precedence; fall back to agent config.
	sysName := system
	if sysName == "" {
		sysName = ag.Metadata.System
	}

	if sysName != "" {
		sys := c.Systems.Get(sysName)
		if sys == nil {
			return "", Model{}, nil, nil, fmt.Errorf("system prompt %q not found", sysName)
		}

		ts.Register("system", sys.Prompt.String())
	} else {
		ts.Register("system", defaultCompositor)
	}

	// Register agent prompt.
	ts.Register("agent", ag.Prompt.String())

	// Register mode prompt (empty if no mode — {{ if .Mode.Name }} guards skip it).
	if mode != "" {
		md := c.Modes.Get(mode)
		if md == nil {
			return "", Model{}, nil, nil, fmt.Errorf("mode %q not found", mode)
		}

		ts.Register("mode", md.Prompt.String())
	} else {
		ts.Register("mode", "")
	}

	// Register autoloaded files — paths are expanded as templates before reading.
	for _, path := range ag.Metadata.Files {
		expandedPath, err := prompts.Prompt(path).Render(data)
		if err != nil {
			return "", Model{}, nil, nil, fmt.Errorf("expanding file path %q: %w", path, err)
		}

		resolved := strings.TrimSpace(expandedPath.String())
		if resolved == "" {
			continue // conditional path, intentionally empty
		}

		content, err := c.ReadAutoload(resolved, paths)
		if err != nil {
			return "", Model{}, nil, nil, err
		}

		templateName := "file:" + resolved
		ts.Register(templateName, content)

		data.Files = append(data.Files, FileEntry{Name: resolved, TemplateName: templateName})
	}

	// Register workspace (AGENTS.md) entries — sorted global before local for stability.
	// Template names use the file path (not just type) to avoid collisions
	// when multiple files share the same type (e.g., two "local" AGENTS.md files).
	for f, md := range c.AgentsMd {
		if !ag.Metadata.AgentsMd.Includes(md.Type) {
			continue
		}

		templateName := "ws:" + f.Path()
		ts.Register(templateName, md.Prompt.String())

		data.Workspace = append(data.Workspace, WorkspaceEntry{
			Type:         fmt.Sprintf("Workspace Instructions: %s", md.Type),
			TemplateName: templateName,
		})
	}

	// Single render pass — the entry point template composes everything.
	rendered, err := ts.Render(data)
	if err != nil {
		return "", Model{}, nil, nil, fmt.Errorf("composing prompt: %w", err)
	}

	return prompts.Prompt(rendered), ag.Metadata.Model, eager, deferred, nil
}

// FilteredHooks returns hooks filtered by the agent's and mode's hook filter settings.
func (c Config) FilteredHooks(agent, mode string) Hooks {
	hks := c.Hooks

	if ag := c.Agents.Get(agent); ag != nil {
		hks = hks.Filtered(ag.Metadata.Hooks.Enabled, ag.Metadata.Hooks.Disabled)
	}

	if mode != "" {
		if md := c.Modes.Get(mode); md != nil {
			hks = hks.Filtered(md.Metadata.Hooks.Enabled, md.Metadata.Hooks.Disabled)
		}
	}

	return hks
}

// Tools returns the filtered tool set for the specified agent and mode combination,
// split into eager (included in request.Tools) and deferred (listed in system prompt
// index, loadable via LoadTools). When no tools are deferred, deferred is nil.
func (c Config) Tools(agent, mode string, rt *Runtime) (tool.Tools, tool.Tools, error) {
	results, err := c.filterTools(agent, mode, rt)
	if err != nil {
		return nil, nil, err
	}

	// Collect included tools.
	all := make(tool.Tools, 0, len(results))
	for _, r := range results {
		if r.included {
			all = append(all, r.tool)
		}
	}

	// Fast path: no deferred config → all eager, zero overhead.
	hasDeferred := len(c.Features.ToolExecution.Deferred) > 0
	if !hasDeferred {
		for _, name := range c.MCPs.Names() {
			if s := c.MCPs.Get(name); s != nil && s.Deferred {
				hasDeferred = true

				break
			}
		}
	}

	if !hasDeferred {
		return all, nil, nil
	}

	// Split into eager and deferred sets.
	var eager, deferred tool.Tools

	var loadedTools map[string]bool

	if rt != nil {
		loadedTools = rt.LoadedTools
	}

	for _, t := range all {
		name := t.Name()

		// Tools already loaded this session stay eager.
		if loadedTools[name] {
			eager = append(eager, t)

			continue
		}

		// Check built-in deferred patterns.
		if wildcard.MatchAny(name, c.Features.ToolExecution.Deferred...) {
			deferred = append(deferred, t)

			continue
		}

		// Check MCP server-level deferred flag.
		if server, _, ok := mcp.ExtractServer(name); ok {
			if s := c.MCPs.Get(server); s != nil && s.Deferred {
				deferred = append(deferred, t)

				continue
			}
		}

		eager = append(eager, t)
	}

	return eager, deferred, nil
}

// CollectEnabled gathers all enabled-tool patterns across global, agent, mode,
// and extra (task-level) filters. Used by opt-in filtering to decide which
// opt-in tools were explicitly requested.
func (c Config) CollectEnabled(agent, mode string, rt *Runtime) []string {
	var all []string

	globalInclude, _ := c.GlobalFilter(rt)

	all = append(all, globalInclude...)

	if ag := c.Agents.Get(agent); ag != nil {
		all = append(all, ag.Metadata.Tools.Enabled...)
	}

	if mode != "" {
		if md := c.Modes.Get(mode); md != nil {
			all = append(all, md.Metadata.Tools.Enabled...)
		}
	}

	if rt != nil && rt.ExtraFilter != nil {
		all = append(all, rt.ExtraFilter.Enabled...)
	}

	return all
}

// toolCondition returns the condition expression for a tool, if any.
// ToolDef condition takes precedence; MCP server condition is the fallback.
func (c Config) toolCondition(name string) string {
	if def, ok := c.ToolDefs[strings.ToLower(name)]; ok && def.Condition != "" {
		return def.Condition
	}

	if server, _, ok := mcp.ExtractServer(name); ok {
		if s := c.MCPs.Get(server); s != nil && s.Condition != "" {
			return s.Condition
		}
	}

	return ""
}

// filterResult holds the outcome of a single tool through the filtering pipeline.
type filterResult struct {
	tool     tool.Tool
	included bool
	reason   string
}

// applyFilter applies an include/exclude filter stage to results.
func applyFilter(results []filterResult, include, exclude []string, disabledReason, notEnabledReason string) {
	for i := range results {
		r := &results[i]

		if !r.included {
			continue
		}

		name := r.tool.Name()

		if wildcard.MatchAny(name, exclude...) {
			r.included = false
			r.reason = disabledReason
		} else if len(include) > 0 && !wildcard.MatchAny(name, include...) {
			r.included = false
			r.reason = notEnabledReason
		}
	}
}

// filterTools applies the 5-stage filtering pipeline to all tools,
// tracking inclusion/exclusion reasons at each stage.
func (c Config) filterTools(agent, mode string, rt *Runtime) ([]filterResult, error) {
	ag := c.Agents.Get(agent)
	if ag == nil {
		return nil, fmt.Errorf("agent %q not found", agent)
	}

	var allTools tool.Tools

	if rt != nil {
		allTools = rt.AllTools
	}

	results := make([]filterResult, len(allTools))
	for i, t := range allTools {
		results[i] = filterResult{tool: t, included: true, reason: "included"}
	}

	// Stage 1: Agent filter.
	applyFilter(results,
		ag.Metadata.Tools.Enabled, ag.Metadata.Tools.Disabled,
		"disabled by agent:"+agent,
		fmt.Sprintf("not in agent:%s enabled list", agent))

	// Stage 2: Mode filter.
	if mode != "" {
		md := c.Modes.Get(mode)
		if md == nil {
			return nil, fmt.Errorf("mode %q not found", mode)
		}

		applyFilter(results,
			md.Metadata.Tools.Enabled, md.Metadata.Tools.Disabled,
			"disabled by mode:"+mode,
			fmt.Sprintf("not in mode:%s enabled list", mode))
	}

	// Stage 3: Extra/task filter.
	if rt != nil && rt.ExtraFilter != nil {
		applyFilter(results,
			rt.ExtraFilter.Enabled, rt.ExtraFilter.Disabled,
			"disabled by task filter",
			"not in task filter enabled list")
	}

	// Stage 4: Condition filter.
	enabled := c.CollectEnabled(agent, mode, rt)

	var condState *condition.State

	if rt != nil {
		condState = rt.ConditionState
	}

	if condState != nil {
		for i := range results {
			r := &results[i]

			if !r.included {
				continue
			}

			name := r.tool.Name()
			cond := c.toolCondition(name)

			if cond != "" && !condition.Check(cond, *condState) && !wildcard.MatchAnyExplicit(name, enabled...) {
				r.included = false
				r.reason = "condition failed: " + cond

				debug.Log("[tools] condition %q failed for %s, excluding", cond, name)
			}
		}
	}

	// Stage 5: Opt-in filter.
	var optInTools []string

	if rt != nil {
		optInTools = rt.OptInTools
	}

	if len(optInTools) > 0 {
		for i := range results {
			r := &results[i]

			if !r.included {
				continue
			}

			name := r.tool.Name()
			if slices.Contains(optInTools, name) && !wildcard.MatchAnyExplicit(name, enabled...) {
				r.included = false
				r.reason = "opt-in (not explicitly enabled)"
			}
		}
	}

	return results, nil
}

// ToolTrace records the inclusion/exclusion status of a single tool
// along with the reason it was filtered.
type ToolTrace struct {
	Name     string
	Included bool
	Reason   string
}

// ToolsWithTrace mirrors the Tools() filtering pipeline but captures
// the reason each tool was included or excluded at each stage.
func (c Config) ToolsWithTrace(agent, mode string, rt *Runtime) ([]ToolTrace, error) {
	results, err := c.filterTools(agent, mode, rt)
	if err != nil {
		return nil, err
	}

	traces := make([]ToolTrace, len(results))
	for i, r := range results {
		traces[i] = ToolTrace{Name: r.tool.Name(), Included: r.included, Reason: r.reason}
	}

	return traces, nil
}

// Options bundles the parameters for New().
type Options struct {
	Homes          []string          // all config dirs in merge order (global first, then --config entries)
	WriteHome      string            // primary home for writes (sessions, debug, auth, plugins)
	GlobalHome     string            // ~/.aura — kept for approval provenance + global auth dir
	LaunchDir      string            // CWD at process start, before --workdir
	WorkDir        string            // CWD after --workdir processing
	WithPlugins    bool              // load plugin definitions
	UnsafePlugins  bool              // enable os/exec in plugins (--unsafe-plugins)
	SetVars        map[string]string // --set template variables for task metadata expansion
	ExtraTaskFiles []string          // additional task file glob patterns (from tasks --files)
}

// New creates a Config by loading the requested parts from standard locations.
// When no parts are given, nothing is loaded — callers must declare what they need.
// Use AllParts() to load everything (equivalent to the old monolithic behavior).
func New(opts Options, parts ...Part) (Config, Paths, error) {
	home := opts.WriteHome

	if home == "" {
		return Config{}, Paths{}, errors.New(
			"no .aura config found in current directory or home directory\n\nRun 'aura init' to create a default configuration",
		)
	}

	debug.Log("[config] loading from %d homes (writeHome=%s)", len(opts.Homes), home)

	fs := Files{}
	if err := fs.Load(opts.WorkDir, opts.LaunchDir, opts.Homes...); err != nil {
		return Config{}, Paths{}, err
	}

	paths := Paths{
		Home:   home,
		Global: opts.GlobalHome,
		Launch: opts.LaunchDir,
		Work:   opts.WorkDir,
	}

	var cfg Config

	loaded := make(map[Part]struct{}, len(parts))

	for _, p := range parts {
		fn, ok := loaders[p]
		if !ok {
			return Config{}, Paths{}, fmt.Errorf("unknown config part: %q", string(p))
		}

		debug.Log("[config] parsing %s...", p)

		if err := fn(&cfg, fs, opts); err != nil {
			return Config{}, Paths{}, err
		}

		loaded[p] = struct{}{}
	}

	// Post-load mutations (gated on loaded set).
	// NOTE: --max-steps and --token-budget are NOT applied here — they're applied
	// in rebuildState() as the final config layer, after agent/mode/task merges.
	// Applying them here would make them part of globalFeatures, where agent
	// frontmatter could silently override them.
	if _, ok := loaded[PartFeatures]; ok {
		if opts.UnsafePlugins {
			cfg.Features.PluginConfig.Unsafe = true
		}
	}

	// Validation — only loaded parts.
	if err := cfg.Validate(loaded); err != nil {
		return Config{}, Paths{}, err
	}

	// Cross-ref validation — each check independently gates on its source+target parts.
	if err := cfg.ValidateCrossRefs(loaded); err != nil {
		return Config{}, Paths{}, err
	}

	debug.Log("[config] loaded: %d providers, %d agents, %d modes, %d prompts, %d mcps, %d tasks",
		len(cfg.Providers), len(cfg.Agents), len(cfg.Modes), len(cfg.Systems), len(cfg.MCPs), len(cfg.Tasks))

	return cfg, paths, nil
}
