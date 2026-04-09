package assistant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/config/override"
	"github.com/idelchi/aura/internal/conversation"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/hooks"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/snapshot"
	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/tools/ask"
	"github.com/idelchi/aura/internal/tools/batch"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/internal/tools/task"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/sandbox"
	"github.com/idelchi/aura/pkg/tokens"
)

// SlashHandler handles slash command input.
// When forward is true, msg should be sent to the LLM as a user message.
type SlashHandler func(ctx context.Context, sctx slash.Context, input string) (msg string, handled, forward bool, err error)

// loopState holds fields that are reset at the start of each processInputs() call.
// Separating these from persistent Assistant state makes the lifecycle explicit.
type loopState struct {
	iteration     int
	toolHistory   []injector.ToolCall
	patchCounts   map[string]int
	pendingEject  bool
	toolsFilter   *config.Tools
	streamStarted bool     // true once the stream callback fires this chat() call
	appendSystem  []string // one-turn system prompt appendages from BeforeChat plugins
}

// resolvedState holds derived state that is recomputed by rebuildState().
// Fields are invalidated together on agent/mode/config changes.
// Note: model is intentionally preserved across rebuildState() — only SetAgent() clears it.
type resolvedState struct {
	// config is the effective runtime configuration snapshot.
	// Built at the end of rebuildState(), read by Status(), SessionMeta(),
	// InjectorState(), TemplateData(), slash commands, and --dry=render.
	config config.Resolved

	model         *model.Model
	compactModel  *model.Model
	titleModel    *model.Model
	thinkingModel *model.Model
	schemaTokens  int
	sandbox       *sandbox.Sandbox
	toolPolicy    *config.ToolPolicy
	hooks         hooks.Runner

	// Guardrail model caches (agents are created on-demand via ResolveAgent).
	guardrail guardrailState
}

// sessionState groups session lifecycle state.
type sessionState struct {
	manager   *session.Manager
	stats     *stats.Stats
	usage     usage.Usage     // cumulative token usage
	dirty     bool            // true after first user message or session resume; gates AutoSave
	approvals map[string]bool // session-scoped tool approval patterns (in-memory only)
}

// streamState groups streaming coordination state.
type streamState struct {
	active  bool
	pending []string
	done    chan struct{}
	cancel  context.CancelFunc
}

// toolState groups tool/plugin infrastructure.
type toolState struct {
	task          *task.Tool       // Task meta-tool (nil when no subagent agents)
	batch         *batch.Tool      // Batch meta-tool
	loaded        map[string]bool  // deferred tools loaded via LoadTools this session
	deferred      tool.Tools       // current deferred set (updated each rebuildState)
	extraFilter   *tool.Filter     // task-level tool filter, persists through rebuildState()
	extraFeatures *config.Features // task-level feature overlay, persists through rebuildState()
	todo          *todo.List
	injectors     *injector.Registry
	plugins       *plugins.Cache
	lsp           *lsp.Manager
	snapshots     *snapshot.Manager
	mcp           []*mcp.Session
}

// toggleState groups boolean runtime toggles.
type toggleState struct {
	verbose          bool
	auto             bool
	doneActive       bool
	doneSignaled     bool
	exitRequested    bool
	sandbox          bool // actual enforcement: requested && IsAvailable()
	sandboxRequested bool // what user wants, independent of kernel support
}

// tokenState groups per-turn token tracking.
type tokenState struct {
	lastInput    int
	lastOutput   int
	lastAPIInput int // raw API-reported input tokens (for delta backfill, not overwritten by estimates)
}

// Assistant orchestrates the conversation loop, tool execution, compaction,
// and state management. Fields are grouped into inner structs by lifecycle:
//
//	loopState     — ephemeral, reset at the start of each processInputs() call.
//	                Tracks per-turn concerns: iteration count, tool history, patch counts.
//
//	resolvedState — derived from config + agent + mode. Invalidated by rebuildState(),
//	                which is called after any mutation (SetAgent, SetMode, SetModel,
//	                ToggleSandbox, /reload, etc.). Contains: resolved model, sandbox,
//	                tool policy, hooks, guardrail model caches, prompt template.
//	                Note: model is intentionally preserved across rebuildState() —
//	                only SetAgent() and SetModel() clear it.
//
//	sessionState  — persistent across turns. Session manager, stats, cumulative usage,
//	                dirty flag, session-scoped approvals. Only reset on session clear.
//
//	streamState   — streaming coordination. Active flag, pending messages, done channel,
//	                cancel func. Reset between streaming turns.
//
//	toolState     — tool infrastructure. Plugin cache, injector registry, LLM estimator,
//	                LSP manager, MCP sessions, snapshot manager, loaded deferred tools.
//	                Rebuilt by rebuildState() for tool list; plugin cache persists.
//
//	toggleState   — runtime toggles (verbose, auto, done, exit, sandbox).
//	                Mutated by Set*/Toggle* methods, persisted across turns.
//
//	tokenState    — per-turn token tracking. Accumulates during a turn, reset next turn.
//
// rebuildState() must be called after any mutation that affects: agent, mode, model,
// thinking, sandbox, tool filters, loaded tools, MCP connections, or feature overrides.
// It rebuilds: tool list, prompt template, sandbox config, guardrail state, hook config,
// and invalidates all 5 feature model caches (compact, title, thinking, guardrail×2).
type Assistant struct {
	// Core dependencies
	agent       *agent.Agent
	cfg         config.Config
	paths       config.Paths    // filesystem context (immutable)
	rt          *config.Runtime // mutable runtime state (persists across reloads)
	events      chan<- ui.Event
	handleSlash SlashHandler
	builder     *conversation.Builder

	// Cached/derived state — invalidated by rebuildState()
	resolved resolvedState

	// Grouped state
	session sessionState
	stream  streamState
	tools   toolState
	toggles toggleState
	tokens  tokenState

	// User-defined --set template variables (session-wide, immutable after construction)
	setVars map[string]string

	// CLI override nodes (--override, --max-steps, --token-budget), applied in rebuildState().
	overrideNodes override.Nodes

	// Noop mode — when set, provider is replaced with this on every agent rebuild.
	noopProvider providers.Provider

	// CLI overrides — stored so SetAgent() can forward them to agent construction.
	cliOverrides agent.Overrides

	// Config reload state
	configOpts config.Options

	// Feature merge base — snapshot of global features, used for per-agent overrides.
	globalFeatures config.Features

	// Task-level workdir override (empty = use configOpts.WorkDir).
	taskWorkDir string

	// Remaining one-off dependencies
	tracker        *filetime.Tracker
	askCallback    func(context.Context, ask.Request) (string, error)
	estimator      *tokens.Estimator
	modelListCache []slash.ProviderModels // cached provider model list from /model command

	// Failover state
	primaryFallbacks []string // fallback agent list from the session's starting agent (immutable)
	failoverIndex    int      // next fallback to try (session-wide, never reset automatically)

	// Per-loop state (reset each processInputs call)
	loop loopState

	// Active context — set at entry to Loop()/ProcessInput(), used by send()
	// to avoid deadlocking on event channel sends when the UI has exited.
	ctx context.Context

	// Shutdown coordination
	done chan struct{}
}

// Params holds all dependencies for constructing an Assistant.
type Params struct {
	Config       config.Config
	Paths        config.Paths
	Runtime      *config.Runtime
	Agent        *agent.Agent
	Events       chan<- ui.Event
	Sessions     *session.Manager
	Todo         *todo.List
	Plugins      *plugins.Cache
	LSP          *lsp.Manager
	Slash        SlashHandler
	Auto         bool
	SetVars      map[string]string
	ConfigOpts   config.Options
	CLIOverrides  agent.Overrides
	OverrideNodes override.Nodes // pre-parsed --override + --max-steps + --token-budget

	// Task tool (nil if no subagent agents).
	TaskTool *task.Tool

	// Batch tool (always present).
	BatchTool *batch.Tool
}

// New creates a fully-wired Assistant. All dependencies are provided at construction —
// no post-construction field assignment needed.
func New(p Params) (*Assistant, error) {
	cfg := p.Config

	// Preserve global features as merge base, then apply agent overrides.
	globalFeatures := cfg.Features

	var primaryFallbacks []string

	if agCfg := cfg.Agents.Get(p.Agent.Name); agCfg != nil {
		primaryFallbacks = agCfg.Metadata.Fallback
	}

	resolved, err := cfg.ResolveFeatures(globalFeatures, p.Agent.Name, p.Agent.Mode)
	if err != nil {
		return nil, fmt.Errorf("resolving features: %w", err)
	}

	cfg.Features = resolved

	registry := injector.New()

	// Register plugin hooks on the injector registry.
	for _, hook := range p.Plugins.Hooks() {
		registry.Register(hook)
	}

	hooksRunner, err := hooks.New(cfg.FilteredHooks(p.Agent.Name, p.Agent.Mode))
	if err != nil {
		return nil, fmt.Errorf("loading hooks: %w", err)
	}

	a := &Assistant{
		ctx:            context.Background(),
		agent:          p.Agent,
		cfg:            cfg,
		paths:          p.Paths,
		rt:             p.Runtime,
		globalFeatures: globalFeatures,
		events:         p.Events,
		handleSlash:    p.Slash,
		resolved: resolvedState{
			sandbox:    buildSandbox(cfg.Features.Sandbox.IsEnabled(), cfg.EffectiveRestrictions(), p.Paths.Work),
			toolPolicy: new(cfg.EffectiveToolPolicy(p.Agent.Name, p.Agent.Mode)),
			hooks:      hooksRunner,
		},
		session: sessionState{
			manager:   p.Sessions,
			stats:     stats.New(),
			approvals: make(map[string]bool),
		},
		stream: streamState{
			pending: []string{},
			done:    make(chan struct{}, 1),
		},
		tools: toolState{
			task:      p.TaskTool,
			batch:     p.BatchTool,
			todo:      p.Todo,
			injectors: registry,
			plugins:   p.Plugins,
			lsp:       p.LSP,
			loaded:    make(map[string]bool),
			snapshots: snapshot.NewManager(p.ConfigOpts.WorkDir),
		},
		toggles: toggleState{
			auto:             p.Auto,
			sandbox:          sandbox.IsAvailable() && cfg.Features.Sandbox.IsEnabled(),
			sandboxRequested: cfg.Features.Sandbox.IsEnabled(),
		},
		setVars:          p.SetVars,
		done:             make(chan struct{}),
		primaryFallbacks: primaryFallbacks,
		loop:             loopState{patchCounts: make(map[string]int)},
		tracker:          filetime.NewTracker(cfg.Features.ToolExecution.ReadBefore.ToPolicy()),
		configOpts:       p.ConfigOpts,
		cliOverrides:     p.CLIOverrides,
	}

	// Store pre-parsed override nodes. Validation already happened in Cache().
	a.overrideNodes = p.OverrideNodes

	// Build initial Resolved for the pre-first-rebuild window
	// (initial Status emit, --dry=render).
	a.resolved.config = config.Resolved{
		Agent:      p.Agent.Name,
		Mode:       p.Agent.Mode,
		Provider:   p.Agent.Model.Provider,
		Model:      p.Agent.Model.Name,
		Think:      p.Agent.Model.Think,
		Context:    p.Agent.Model.Context,
		Generation: p.Agent.Model.Generation,
		Thinking:   p.Agent.Thinking,
		Features:   cfg.Features,
		Sandbox:    sandbox.IsAvailable() && cfg.Features.Sandbox.IsEnabled(),
		Verbose:    false,
		Auto:       p.Auto,
		Done:       false,
	}

	a.resolved.hooks.OnStart = func(name string) {
		a.send(ui.SpinnerMessage{Text: name + " running…"})
	}

	estimator, err := cfg.Features.Estimation.NewEstimator()
	if err != nil {
		return nil, fmt.Errorf("creating estimator: %w", err)
	}

	a.estimator = estimator
	a.estimator.SetDebug(debug.Log)

	// Builder is created after the struct so a.send (which captures a.ctx by reference)
	// can serve as the event sink — providing context-aware, non-blocking event delivery.
	systemTokens := a.estimator.EstimateLocal(p.Agent.Prompt)

	a.builder = conversation.NewBuilder(a.send, p.Agent.Prompt, systemTokens, func(ctx context.Context, s string) int {
		return a.estimator.Estimate(ctx, s)
	})

	a.resolved.schemaTokens = a.estimator.EstimateLocal(p.Agent.Tools.Schemas().Render())

	return a, nil
}

// rebuildInjectors recreates the injector registry with current effective features.
// Preserves fired state so once-per-session injectors don't re-fire.
func (a *Assistant) rebuildInjectors() {
	firedState := a.tools.injectors.FiredState()

	registry := injector.New()

	// Re-register plugin hooks from cached interpreters.
	for _, hook := range a.tools.plugins.Hooks() {
		registry.Register(hook)
	}

	registry.RestoreFiredState(firedState)

	a.tools.injectors = registry
}

// Close releases all resources held by the assistant.
func (a *Assistant) Close() {
	for _, t := range a.rt.AllTools {
		if cl, ok := t.(tool.Closer); ok {
			cl.Close()
		}
	}

	if a.tools.snapshots != nil {
		if err := a.tools.snapshots.Prune(); err != nil {
			debug.Log("[snapshot] prune: %v", err)
		}
	}

	a.closeMCPSessions()

	if a.tools.lsp != nil {
		a.tools.lsp.StopAll()
	}

	a.tools.plugins.Close()
}

// Paths returns the filesystem paths context (immutable after construction).
func (a *Assistant) Paths() config.Paths { return a.paths }

// Runtime returns the mutable runtime state that persists across reloads.
func (a *Assistant) Runtime() *config.Runtime { return a.rt }

// Done returns a channel that is closed when Loop() returns.
func (a *Assistant) Done() <-chan struct{} {
	return a.done
}

// SessionMeta builds a complete session.Meta snapshot including all runtime
// toggles and session-scoped state. Used by AutoSave, /save, and /fork.
func (a *Assistant) SessionMeta() session.Meta {
	r := a.resolved.config

	m := session.Meta{
		Agent:    r.Agent,
		Mode:     r.Mode,
		Think:    r.Think.AsString(),
		Model:    r.Model,
		Provider: r.Provider,
		Verbose:  r.Verbose,
		Sandbox:  r.Sandbox,

		LoadedTools:      loadedToolNames(a.tools.loaded),
		SessionApprovals: a.session.approvals,
		Stats:            a.session.stats,
		CumulativeUsage:  &a.session.usage,
	}

	// Only persist non-default policy (nil = use config default on resume).
	p := a.tracker.Policy()
	if p != tool.DefaultReadBeforePolicy() {
		m.ReadBeforePolicy = &p
	}

	return m
}

// AutoSave persists the current session state without title generation.
// Safe to call during shutdown (no LLM calls).
func (a *Assistant) AutoSave() error {
	if a.session.manager == nil || !a.session.dirty {
		return nil
	}

	title := a.session.manager.ActiveTitle()

	meta := a.SessionMeta()

	_, err := a.session.manager.Save(title, meta, a.builder.History(), a.tools.todo)

	return err
}

// ResetTokens clears cached token counts, forcing Status() to re-estimate.
func (a *Assistant) ResetTokens() {
	a.tokens.lastInput = 0
	a.tokens.lastOutput = 0
	a.tokens.lastAPIInput = 0
	a.session.usage = usage.Usage{}
}

// WireEstimation registers the provider's Estimate endpoint as the native
// estimation function on the Estimator. Idempotent — safe to call on every chat() turn.
// Before model resolution, native falls back to local (correct for construction-time estimates).
func (a *Assistant) WireEstimation() {
	if a.cfg.Features.Estimation.Method != "native" || a.resolved.model == nil {
		return
	}

	a.estimator.UseNative(func(ctx context.Context, text string) (int, error) {
		req := request.Request{
			Model:         a.resolved.model.Deref(),
			ContextLength: int(a.ContextLength()),
		}

		count, err := a.agent.Provider.Estimate(ctx, req, text)
		if err != nil {
			if errors.Is(err, providers.ErrContextExhausted) {
				return count, nil // capped at numCtx, still valid
			}

			debug.Log("[ERROR] provider.Estimate failed: %v — falling back to local estimator", err)
			a.send(ui.CommandResult{
				Command: "estimation",
				Message: fmt.Sprintf("native token estimation failed: %v — using local estimator", err),
			})

			return 0, err
		}

		return count, nil
	})
}

// ResolveModel lazily resolves and caches a model for an agent.
// On first call (*cache == nil), it fetches the model from the provider and stores it.
// Subsequent calls return the cached value.
func (a *Assistant) ResolveModel(
	ctx context.Context,
	label string,
	ag *agent.Agent,
	cache **model.Model,
) (model.Model, error) {
	if *cache != nil {
		return **cache, nil
	}

	debug.Log("[%s] resolving model=%s provider=%s", label, ag.Model.Name, ag.Model.Provider)

	m, err := ag.Provider.Model(ctx, ag.Model.Name)
	if err != nil {
		return model.Model{}, fmt.Errorf("resolving %s model: %w", label, err)
	}

	*cache = &m

	debug.Log("[%s] resolved model: contextLength=%d", label, int(m.ContextLength))

	return m, nil
}

// InjectorState creates a State snapshot for injectors.
func (a *Assistant) InjectorState() *injector.State {
	// Session identity (nil-safe — sessions can be nil in headless mode).
	var sessionID, sessionTitle string

	if a.session.manager != nil {
		sessionID = a.session.manager.ActiveID()
		sessionTitle = a.session.manager.ActiveTitle()
	}

	r := a.resolved.config

	return &injector.State{
		Iteration:   a.loop.iteration,
		ToolHistory: a.loop.toolHistory,
		Todo: injector.TodoState{
			Pending:    len(a.tools.todo.FindPending()),
			InProgress: len(a.tools.todo.FindInProgress()),
			Total:      a.tools.todo.Len(),
		},
		Mode:        r.Mode,
		Auto:        r.Auto,
		PatchCounts: a.loop.patchCounts,
		Tokens: injector.TokenSnapshot{
			Estimate: a.Tokens(),
			LastAPI:  a.tokens.lastAPIInput,
			Percent:  a.Status().Tokens.Percent,
			Max:      int(a.ContextLength()),
		},
		MessageCount: a.builder.Len(),
		Stats:        a.session.stats.Snapshot(),
		Model:        a.resolved.model.Deref(),
		Agent:        r.Agent,
		Workdir:      a.effectiveWorkDir(),
		DoneActive:   r.Done,
		MaxSteps:     r.Features.ToolExecution.MaxSteps,

		Session: injector.SessionState{
			ID:    sessionID,
			Title: sessionTitle,
		},
		Provider:  r.Provider,
		ThinkMode: r.Think.String(),
		Sandbox: injector.SandboxState{
			Enabled:   r.Sandbox,
			Requested: a.toggles.sandboxRequested,
		},
		ReadBeforeWrite: a.tracker.Policy().Write,
		ShowThinking:    r.Verbose,
		Compaction: injector.CompactionState{
			Enabled: r.Features.Compaction.Threshold > 0 || r.Features.Compaction.MaxTokens > 0,
		},
		AvailableTools: a.agent.Tools.Names(),
		LoadedTools:    loadedToolNames(a.tools.loaded),
		Turns:          a.builder.Turns(),
		SystemPrompt:   a.builder.SystemPrompt(),
		MCPServers:     mcpServerNames(a.tools.mcp),
		Vars:           a.setVars,
	}
}

// Track records a tool invocation for injector state.
func (a *Assistant) Track(name string, args map[string]any, result, toolErr string, duration time.Duration) {
	a.loop.toolHistory = append(a.loop.toolHistory, injector.ToolCall{
		Name: name, Args: args, Result: result, Error: toolErr, Duration: duration,
	})

	// Track patches for RepeatedPatch injector.
	// Only increment — turn-level reset happens in loopState initialization.
	if name == "Patch" || name == "Edit" {
		if path, ok := args["path"].(string); ok {
			a.loop.patchCounts[path]++
		}
	}
}

// send emits a UI event, aborting silently if the active context is cancelled.
// This prevents deadlocks when the UI goroutine has exited and nobody is reading
// from the events channel.
func (a *Assistant) send(event ui.Event) {
	select {
	case a.events <- event:
	case <-a.ctx.Done():
	}
}

// Send emits a UI event. Public wrapper for use by external callbacks (e.g. ask tool).
func (a *Assistant) Send(event ui.Event) { a.send(event) }

// SetWorkDir overrides the effective working directory for task workdir: support.
// Rebuilds system prompt and sandbox to reflect the new directory.
func (a *Assistant) SetWorkDir(wd string) error {
	a.taskWorkDir = wd

	return a.rebuildState()
}

// effectiveWorkDir returns the task workdir override if set, otherwise configOpts.WorkDir.
func (a *Assistant) effectiveWorkDir() string {
	if a.taskWorkDir != "" {
		return a.taskWorkDir
	}

	return a.configOpts.WorkDir
}

// WorkDir returns the effective working directory (post --workdir resolution).
func (a *Assistant) WorkDir() string { return a.effectiveWorkDir() }

// injectMessages applies a batch of injections to the conversation.
// Sets pendingEject if any injection is marked for one-turn-only.
// Returns the last non-nil tool filter from the batch (nil = no filtering).
func (a *Assistant) injectMessages(injections []injector.Injection) *config.Tools {
	var toolsFilter *config.Tools

	for _, inj := range injections {
		content := inj.Prefix + inj.Content

		if !inj.DisplayOnly {
			a.builder.InjectMessage(a.ctx, inj.Role, content)
		}

		a.send(ui.SyntheticInjected{
			Header:  inj.DisplayHeader(),
			Content: inj.DisplayContent(),
			Role:    string(inj.Role),
		})

		if !inj.DisplayOnly && inj.Eject {
			a.loop.pendingEject = true
		}

		if inj.Tools != nil {
			toolsFilter = inj.Tools
		}
	}

	return toolsFilter
}

// buildSandbox constructs a Sandbox from merged restrictions for lightweight path checks.
func buildSandbox(enabled bool, restrictions config.Restrictions, workDir string) *sandbox.Sandbox {
	if !enabled {
		return nil
	}

	sb := &sandbox.Sandbox{WorkDir: workDir}
	sb.AddReadOnly(restrictions.ReadOnly...)
	sb.AddReadWrite(restrictions.ReadWrite...)

	if !sb.HasRules() {
		return nil
	}

	return sb
}

