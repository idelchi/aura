package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/assistant"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/config/override"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/slash/commands"
	"github.com/idelchi/aura/internal/subagent"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/tools/ask"
	"github.com/idelchi/aura/internal/tools/assemble"
	"github.com/idelchi/aura/internal/tools/batch"
	"github.com/idelchi/aura/internal/tools/task"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/internal/ui/headless"
	"github.com/idelchi/aura/internal/ui/simple"
	"github.com/idelchi/aura/internal/ui/tui"
	"github.com/idelchi/aura/pkg/cache"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/providers/noop"
	"github.com/idelchi/aura/pkg/providers/registry"
	"github.com/idelchi/aura/pkg/spinner"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
	"github.com/idelchi/godyl/pkg/pretty"
)

// ErrUserAbort is the cancellation cause set when the user sends SIGINT/SIGTERM.
// Programmatic cancellations (timeouts, parent context) use different causes,
// allowing downstream code to distinguish user intent from infrastructure errors.
var ErrUserAbort = errors.New("user abort")

// SessionFunc is the mode-specific work done within a session.
// It receives a cancellable context, the fully-wired assistant, and the UI.
// For interactive mode, this runs Loop() + UI.Run().
// For batch mode, this runs ProcessInput() calls sequentially.
type SessionFunc func(ctx context.Context, cancel context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error

// RunSession handles the full assistant session lifecycle:
// flags, config, UI, assistant, MCP, signal handling, graceful shutdown, auto-save.
// The work callback does the mode-specific processing.
func RunSession(flags Flags, makeUI func(Flags) (ui.UI, error), work SessionFunc) error {
	if done, err := handleEarlyExits(flags); done || err != nil {
		return err
	}

	// Context + signal handling
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel(ErrUserAbort)
	}()

	// Initialize debug logger early.
	debug.Init(flags.WriteHome(), flags.Debug)

	defer debug.Close()

	doneStartup := debug.Span("startup")

	sp := spinner.New("Starting...")

	sp.Start()
	defer sp.Stop()

	// Create UI
	doneUI := debug.Span("creating UI")

	u, err := makeUI(flags)
	if err != nil {
		return fmt.Errorf("creating UI: %w", err)
	}

	doneUI()

	// Create assistant (no MCP yet — instant, no network)
	doneAssistant := debug.Span("creating assistant")

	asst, slashRegistry, err := NewAssistant(flags, u.Events(), sp.Update)
	if err != nil {
		return fmt.Errorf("creating assistant: %w", err)
	}
	defer asst.Close()

	doneAssistant()

	if flags.Dry == "render" {
		doneStartup()

		w := flags.Writer
		r := asst.Resolved()
		setVars := asst.TemplateVars()

		fmt.Fprintln(w, "=== Dry Run (render) ===")
		fmt.Fprintf(w, "Agent:    %s (%s)\n", r.Agent, r.Model)
		fmt.Fprintf(w, "Provider: %s\n", r.Provider)
		fmt.Fprintf(w, "Mode:     %s\n", r.Mode)

		if len(setVars) > 0 {
			fmt.Fprint(w, "Set vars: ")

			first := true

			for k, v := range setVars {
				if !first {
					fmt.Fprint(w, ", ")
				}

				fmt.Fprintf(w, "%s=%s", k, v)

				first = false
			}

			fmt.Fprintln(w)
		}

		fmt.Fprintln(w)
		fmt.Fprintln(w, "System prompt:")
		fmt.Fprintln(w, "---")
		fmt.Fprintln(w, asst.SystemPrompt())
		fmt.Fprintln(w, "---")

		return nil
	}

	// Emit resolved status immediately so the UI shows the correct agent name.
	u.Events() <- ui.StatusChanged{Status: asst.Status()}

	u.Events() <- ui.DisplayHintsChanged{Hints: asst.DisplayHints()}

	// Wire slash command hints (no-op for headless/simple, active for TUI)
	if slashRegistry != nil {
		u.SetHintFunc(slashRegistry.HintFor)
	}

	// Wire working directory for directive autocomplete
	u.SetWorkdir(asst.WorkDir())

	// Resolve --continue/--resume and load session history.
	if err := resumeSession(ctx, flags, asst, u.Events()); err != nil {
		return err
	}

	// Connect MCP servers (blocks until all are connected or failed)
	sp.Update("Connecting MCP servers...")

	doneMCP := debug.Span("connecting MCP")

	ConnectMCP(ctx, asst, u.Events())
	doneMCP()

	// Rebuild state so MCP tools enter the active set before the loop starts.
	// Without this, MCP tools only appear after the first user input triggers reloadConfig.
	if err := asst.RebuildState(); err != nil {
		debug.Log("[session] post-MCP rebuild: %v", err)
	}

	sp.Stop()
	doneStartup()

	// Run mode-specific work
	err = work(ctx, cancel, asst, u)

	// Ensure context is cancelled (UI may have exited before signal)
	cancel(nil)

	// Wait for assistant loop to reach a safe point
	select {
	case <-asst.Done():
	case <-time.After(3 * time.Second):
	}

	// Auto-save session state
	if saveErr := asst.AutoSave(); saveErr != nil {
		fmt.Fprintf(os.Stderr, "auto-save error: %v\n", saveErr)

		if err == nil {
			err = fmt.Errorf("session auto-save failed: %w", saveErr)
		}
	}

	return err
}

// InteractiveUI creates a TUI or Simple UI based on flags.
func InteractiveUI(flags Flags) (ui.UI, error) {
	home := flags.Home
	if home == "" {
		home = flags.WriteHome()
	}

	var historyPath string

	if home != "" {
		historyPath = file.New(home, "history").Path()
		folder.New(file.New(historyPath).Dir()).Create()
	}

	if flags.Simple {
		return simple.New(ui.Status{Agent: flags.Agent}, historyPath), nil
	}

	var output *os.File

	if flags.Mirror != nil {
		output = flags.Mirror.OrigStdout()
	}

	return tui.New(ui.Status{Agent: flags.Agent}, historyPath, output), nil
}

// HeadlessUI creates a headless UI for non-interactive commands.
func HeadlessUI(_ Flags) (ui.UI, error) {
	return headless.New(), nil
}

// NewAssistant creates a configured assistant with slash commands wired.
// MCP connections are NOT made here — they happen in the background after the UI starts.
// Returns the assistant and the slash registry (for hint resolution by the TUI).
func NewAssistant(
	flags Flags,
	events chan<- ui.Event,
	onProgress func(string),
) (*assistant.Assistant, *slash.Registry, error) {
	progress := func(msg string) {
		if onProgress != nil {
			onProgress(msg)
		}
	}

	opts := flags.ConfigOptions()

	opts.LaunchDir = LaunchDir
	opts.WorkDir = WorkDir
	opts.WithPlugins = !flags.WithoutPlugins
	opts.UnsafePlugins = flags.UnsafePlugins
	opts.ExtraTaskFiles = flags.Tasks.Files

	// Convert dedicated flags to override strings so they flow through the
	// same mechanism. Prepended (not appended) so --override wins (last wins).
	var dedupOverrides []string

	if flags.IsSet("max-steps") {
		dedupOverrides = append(dedupOverrides, fmt.Sprintf("features.tools.max_steps=%d", flags.MaxSteps))
	}

	if flags.IsSet("token-budget") {
		dedupOverrides = append(dedupOverrides, fmt.Sprintf("features.tools.token_budget=%d", flags.TokenBudget))
	}

	allOverrides := append(dedupOverrides, flags.Override...)

	progress("Loading config...")

	doneConfig := debug.Span("  loading config")

	cfg, paths, err := config.New(opts, config.AllParts()...)
	if err != nil {
		return nil, nil, err
	}

	doneConfig()

	progress("Refreshing providers...")

	appCache := cache.New(flags.WriteHome(), flags.NoCache)

	doneRefresh := debug.Span("  refreshing registry")

	registry.Refresh(context.Background(), appCache.Domain("catwalk"))
	doneRefresh()

	// Resolve default agent when --agent is not explicitly set.
	if flags.Agent == "" {
		resolved, err := config.ResolveDefault(cfg.Agents, flags.Homes())
		if err != nil {
			return nil, nil, err
		}

		flags.Agent = resolved.Metadata.Name
	}

	// Load plugins early so plugin tools are available in the tool registry.
	var pluginCache *plugins.Cache

	if opts.WithPlugins {
		progress("Loading plugins...")

		donePlugins := debug.Span("  loading plugins")

		pluginCache, err = plugins.LoadAll(cfg.Plugins, cfg.Features.PluginConfig, paths.Home)
		if err != nil {
			return nil, nil, fmt.Errorf("loading plugins: %w", err)
		}

		donePlugins()
	}

	// Create LSP manager if configured.
	var lspManager *lsp.Manager

	if len(cfg.LSPServers) > 0 {
		progress("Starting LSP...")

		doneLSP := debug.Span("  starting LSP")

		lspManager = lsp.NewManager(lsp.Config{Servers: map[string]lsp.Server(cfg.LSPServers)}, WorkDir)

		doneLSP()
	}

	progress("Registering tools...")

	// Build tool registry (base + runtime + task tool) and apply filters.
	todoList := todo.New()

	rt := &config.Runtime{
		WithPlugins:      opts.WithPlugins,
		UnsafePlugins:    opts.UnsafePlugins,
		DisplayProviders: flags.Providers,
		IncludeTools:     flags.IncludeTools,
		ExcludeTools:     flags.ExcludeTools,
		IncludeMCPs:      flags.IncludeMCPs,
		ExcludeMCPs:      flags.ExcludeMCPs,
	}

	taskTool, batchTool, err := buildToolRegistry(&cfg, paths, rt, flags, todoList, events, pluginCache, lspManager)
	if err != nil {
		return nil, nil, err
	}

	// Build CLI overrides for agent construction.
	overrides, err := buildOverrides(flags)
	if err != nil {
		return nil, nil, err
	}

	// Parse + validate all overrides (--max-steps, --token-budget, --override)
	// into cached YAML nodes against OverrideTarget. One validation path.
	var overrideTarget config.OverrideTarget

	overrideNodes, err := override.Cache(&overrideTarget, allOverrides)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid --override: %w", err)
	}

	// Extract model fields for agent.Overrides (provider reconstruction in agent.New).
	// --override model.* wins over --model/--provider/--think when both set.
	m := overrideTarget.Model

	if m.Name != "" {
		overrides.Model = &m.Name
	}

	if m.Provider != "" {
		overrides.Provider = &m.Provider
	}

	if m.Think.Value != nil {
		overrides.Think = &m.Think
	}

	if m.Context != 0 {
		overrides.Context = &m.Context
	}

	if m.Generation != nil {
		overrides.Generation = m.Generation
	}

	if len(cfg.Agents) == 0 {
		return nil, nil, fmt.Errorf(
			"no agents configured in %s/config/agents\n\nRun 'aura init --dir %s' to create a default configuration",
			paths.Home, paths.Home,
		)
	}

	progress("Building agent...")

	doneAgent := debug.Span("  building primary agent")

	ag, err := agent.New(cfg, paths, rt, flags.Agent, overrides)
	if err != nil {
		return nil, nil, err
	}

	doneAgent()

	// Noop mode flag — passed to assistant, which applies it on every agent rebuild.
	noopMode := flags.Dry == "noop"

	// Build slash command registry
	slashRegistry := slash.New(commands.All...)

	for _, cmd := range slash.FromCustomCommands(cfg.Commands) {
		if _, exists := slashRegistry.Lookup(cmd.Name); exists {
			events <- ui.CommandResult{
				Command: "startup",
				Error:   fmt.Errorf("custom command %s conflicts with built-in, skipping", cmd.Name),
			}

			continue
		}

		slashRegistry.Register(cmd)
	}

	// Register plugin commands (lowest priority: built-in > custom > plugin).
	for _, cmd := range pluginCache.Commands() {
		if _, exists := slashRegistry.Lookup(cmd.Name); exists {
			events <- ui.CommandResult{
				Command: "startup",
				Error:   fmt.Errorf("plugin command %s conflicts with existing command, skipping", cmd.Name),
			}

			continue
		}

		slashRegistry.Register(cmd)
	}

	// Wire session persistence
	sessionDir := folder.New(flags.WriteHome(), "sessions").Path()
	sessionMgr := session.NewManager(session.NewStore(sessionDir))

	progress("Wiring assistant...")

	doneAsstNew := debug.Span("  wiring assistant")

	p := assistant.Params{
		Config:       cfg,
		Paths:        paths,
		Runtime:      rt,
		Agent:        ag,
		Events:       events,
		Todo:         todoList,
		Sessions:     sessionMgr,
		Slash:        slashRegistry.Handle,
		Auto:         flags.Auto,
		SetVars:      flags.Set,
		ConfigOpts:   opts,
		Plugins:      pluginCache,
		LSP:          lspManager,
		CLIOverrides:  overrides,
		OverrideNodes: overrideNodes,
	}

	if taskTool != nil {
		p.TaskTool = taskTool
	}

	p.BatchTool = batchTool

	asst, err := assistant.New(p)
	if err != nil {
		return nil, nil, fmt.Errorf("creating assistant: %w", err)
	}

	doneAsstNew()

	if noopMode {
		asst.SetNoop(noop.New())
	}

	// Wire Task tool callback — must happen after assistant construction.
	if taskTool != nil {
		taskTool.Run = func(ctx context.Context, agentName, prompt string) (subagent.Result, error) {
			return asst.RunSubagent(ctx, agentName, prompt)
		}
	}

	// Wire Batch tool callbacks — must happen after assistant construction.
	batchTool.Run = func(ctx context.Context, toolName string, args map[string]any) (string, error) {
		return asst.ExecuteSubTool(ctx, toolName, args)
	}

	batchTool.IsParallel = func(name string) bool {
		return asst.IsToolParallel(name)
	}

	return asst, slashRegistry, nil
}

// handleEarlyExits processes --show, --print, --print-env flags and validates --dry mode.
// Returns (true, nil) when the flag was handled and the caller should return,
// (false, nil) for pass-through, or (false, err) for validation failures.
func handleEarlyExits(flags Flags) (bool, error) {
	if flags.Show {
		cfg, _, err := config.New(flags.ConfigOptions(), config.AllParts()...)
		if err != nil {
			return false, err
		}

		pretty.PrintYAML(cfg)

		return true, nil
	}

	if flags.Print {
		cfg, _, err := config.New(flags.ConfigOptions(), config.AllParts()...)
		if err != nil {
			return false, err
		}

		fmt.Fprint(flags.Writer, cfg.Summary(flags.Homes()).Display())

		return true, nil
	}

	if flags.PrintEnv {
		for _, e := range flags.ToEnv() {
			fmt.Fprintln(flags.Writer, e)
		}

		return true, nil
	}

	switch flags.Dry {
	case "render", "noop", "":
		// valid
	default:
		return false, fmt.Errorf("unknown --dry mode %q (valid: render, noop)", flags.Dry)
	}

	return false, nil
}

// resumeSession resolves --continue to --resume (most recent session) and loads the session.
// The flags parameter is by value — internal mutation of flags.Resume stays local.
func resumeSession(ctx context.Context, flags Flags, asst *assistant.Assistant, events chan<- ui.Event) error {
	if flags.Continue {
		if flags.Resume != "" {
			return errors.New("cannot use --resume and --continue together")
		}

		result, err := asst.SessionManager().List()
		if err != nil {
			return fmt.Errorf("listing sessions for --continue: %w", err)
		}

		if len(result.Summaries) == 0 {
			return errors.New("no sessions to continue")
		}

		flags.Resume = result.Summaries[0].ID
	}

	if flags.Resume != "" {
		resolved, err := asst.SessionManager().Find(flags.Resume)
		if err != nil {
			return fmt.Errorf("resolving session: %w", err)
		}

		sess, err := asst.SessionManager().Resume(resolved)
		if err != nil {
			return fmt.Errorf("resuming session: %w", err)
		}

		warnings := asst.ResumeSession(ctx, sess)
		for _, w := range warnings {
			events <- ui.CommandResult{Command: "resume", Message: w, Level: ui.LevelWarn}
		}
	}

	return nil
}

// buildToolRegistry assembles the complete tool set (base + runtime + task tool),
// applies tool defs, opt-in flags, provider filtering, and global filters.
// cfg is passed by pointer because this function may mutate Agents.
// rt is passed by pointer because this function sets AllTools, OptInTools, and related runtime fields.
func buildToolRegistry(
	cfg *config.Config,
	paths config.Paths,
	rt *config.Runtime,
	flags Flags,
	todoList *todo.List,
	events chan<- ui.Event,
	pluginCache *plugins.Cache,
	lspManager *lsp.Manager,
) (*task.Tool, *batch.Tool, error) {
	doneTools := debug.Span("  registering tools")

	estimator, err := cfg.Features.Estimation.NewEstimator()
	if err != nil {
		return nil, nil, fmt.Errorf("creating estimator: %w", err)
	}

	rt.Estimator = estimator

	result, err := assemble.Tools(assemble.Params{
		Config:      *cfg,
		Paths:       paths,
		Runtime:     rt,
		TodoList:    todoList,
		Events:      events,
		PluginCache: pluginCache,
		LSPManager:  lspManager,
		Estimate:    estimator.EstimateLocal,
	})
	if err != nil {
		return nil, nil, err
	}

	doneTools()

	// Assemble opt-in tool names from features config + plugin flags.
	rt.OptInTools = append(rt.OptInTools, cfg.Features.ToolExecution.OptIn...)

	if pluginCache != nil {
		for _, pt := range pluginCache.Tools() {
			if pt.IsOptIn() {
				rt.OptInTools = append(rt.OptInTools, pt.Name())
			}
		}
	}

	// Filter agents whose provider is not in --providers.
	if len(rt.DisplayProviders) > 0 {
		providers := rt.DisplayProviders

		cfg.Agents.RemoveIf(func(a config.Agent) bool { return !a.HasProvider(providers) })

		if cfg.Agents.Get(flags.Agent) == nil {
			return nil, nil, fmt.Errorf(
				"agent %q uses a provider not in --providers %v",
				flags.Agent, rt.DisplayProviders,
			)
		}
	}

	// Apply global tool filters (CLI flags override config defaults) before agent/mode filtering.
	if include, exclude := cfg.GlobalFilter(rt); len(include) > 0 || len(exclude) > 0 {
		rt.AllTools = rt.AllTools.Filtered(include, exclude)
	}

	return result.Task, result.Batch, nil
}

// buildOverrides constructs agent.Overrides from CLI flags.
// Pure function — reads flag values and IsSet checks, returns the overrides struct.
func buildOverrides(flags Flags) (agent.Overrides, error) {
	var overrides agent.Overrides

	if flags.IsSet("agent") {
		overrides.Agent = &flags.Agent
	}

	if flags.IsSet("mode") {
		overrides.Mode = &flags.Mode
	}

	if flags.IsSet("system") {
		overrides.System = &flags.System
	}

	if flags.IsSet("think") {
		tv, err := thinking.ParseValue(flags.Think)
		if err != nil {
			return agent.Overrides{}, err
		}

		overrides.Think = &tv
	}

	if flags.IsSet("provider") {
		overrides.Provider = &flags.Provider
	}

	if flags.IsSet("model") {
		overrides.Model = &flags.Model
	}

	overrides.Vars = flags.Set

	return overrides, nil
}

// RunInteractive is the SessionFunc for interactive commands (run, web).
// It wires the Ask tool, starts the assistant loop, and blocks on the UI.
func RunInteractive(ctx context.Context, cancel context.CancelCauseFunc, asst *assistant.Assistant, u ui.UI) error {
	if err := asst.EnableAsk(func(ctx context.Context, req ask.Request) (string, error) {
		if asst.Resolved().Auto {
			if len(req.Options) > 0 {
				return req.Options[0].Label, nil
			}

			return "proceed", nil
		}

		respCh := make(chan string, 1)
		uiOptions := make([]ui.AskOption, len(req.Options))

		for i, o := range req.Options {
			uiOptions[i] = ui.AskOption{Label: o.Label, Description: o.Description}
		}

		asst.Send(ui.AskRequired{
			Question:    req.Question,
			Options:     uiOptions,
			MultiSelect: req.MultiSelect,
			Response:    respCh,
		})

		select {
		case resp := <-respCh:
			if resp == "" {
				return "", errors.New("user dismissed the question")
			}

			return resp, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}); err != nil {
		return fmt.Errorf("enabling ask tool: %w", err)
	}

	u.Events() <- ui.StatusChanged{Status: asst.Status()}

	u.Events() <- ui.DisplayHintsChanged{Hints: asst.DisplayHints()}

	go func() {
		if err := asst.Loop(ctx, u.Input(), u.Actions(), u.Cancel()); err != nil {
			if !errors.Is(err, context.Canceled) {
				debug.Log("[session] loop error: %v", err)
				cancel(err)

				select {
				case u.Events() <- ui.CommandResult{Error: err}:
				case <-ctx.Done():
				}
			}
		}
	}()

	if err := u.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "UI error: %v\n", err)
	}

	return nil
}

// Run is the Action handler for the root command (interactive mode).
func Run() error {
	return RunSession(GetFlags(), InteractiveUI, RunInteractive)
}
