package assistant

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/internal/plugins"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/slash/commands"
	"github.com/idelchi/aura/internal/tools/assemble"
	"github.com/idelchi/aura/internal/ui"
)

// reloadConfig re-reads all config from disk and rebuilds derived state
// (tools, injectors, slash registry, prompt, sandbox).
// Preserves MCP tools from existing sessions without reconnecting.
// Called per-turn from processInputs() and as the first phase of Reload().
func (a *Assistant) reloadConfig(existing *config.Config) error {
	var (
		newCfg config.Config
		err    error
	)

	if existing != nil {
		newCfg = *existing
	} else {
		newCfg, _, err = config.New(a.configOpts, config.AllParts()...)
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
	}

	// Store the clean global base, then select plugins using all effective layers.
	globalFeatures := newCfg.Features
	newCfg.Features, err = a.effectiveFeatures(newCfg, globalFeatures)
	if err != nil {
		return err
	}
	if _, err := a.tools.plugins.Select(newCfg.Features.PluginConfig); err != nil {
		return err
	}

	// Rebuild estimator from merged config (method/divisor/encoding may have changed,
	// including task-level overrides applied above).
	newEstimator, err := newCfg.Features.Estimation.NewEstimator()
	if err != nil {
		return fmt.Errorf("creating estimator: %w", err)
	}

	newEstimator.SetDebug(debug.Log)

	a.rt.Estimator = newEstimator
	a.estimator = newEstimator
	if err := a.rebuildTools(newCfg); err != nil {
		return err
	}
	a.cfg = newCfg
	a.globalFeatures = globalFeatures

	// Keep the resolved model: config reload does not change the selected agent.
	// Unchanged plugin interpreters also retain their state across turns.
	return a.rebuildState()
}

// rebuildTools uses the canonical assembly pipeline, preserving Task/Batch callbacks
// and permitted MCP sessions. Reassembly restores built-ins when an overriding
// plugin is deselected as well as removing that plugin's exported tools.
func (a *Assistant) rebuildTools(cfg config.Config) error {
	// Rebuild tools — re-inject existing Task/Batch rather than creating new ones.
	if _, err := assemble.Tools(assemble.Params{
		Config:        cfg,
		Paths:         a.paths,
		Runtime:       a.rt,
		TodoList:      a.tools.todo,
		Events:        a.events,
		PluginCache:   a.tools.plugins,
		LSPManager:    a.tools.lsp,
		Estimate:      a.estimator.EstimateLocal,
		ExistingTask:  a.tools.task,
		ExistingBatch: a.tools.batch,
	}); err != nil {
		return fmt.Errorf("rebuilding tools: %w", err)
	}

	// Re-inject MCP tools from sessions that pass the current filter.
	// Sessions excluded by the new filter are closed (resource cleanup).
	mcpInclude, mcpExclude := cfg.MCPFilter(a.rt)
	filteredMCPs := config.FilterMCPs(cfg.MCPs, mcpInclude, mcpExclude)

	var keptSessions []*mcp.Session

	for _, s := range a.tools.mcp {
		if filteredMCPs.Get(s.Name) != nil {
			for _, t := range s.Tools() {
				a.rt.AllTools.Add(t)
			}

			keptSessions = append(keptSessions, s)
		} else {
			if err := s.Close(); err != nil {
				debug.Log("[mcp] close filtered-out %s: %v", s.Name, err)
			}
		}
	}

	a.tools.mcp = keptSessions

	if len(a.rt.DisplayProviders) > 0 {
		providers := a.rt.DisplayProviders
		cfg.Agents.RemoveIf(func(a config.Agent) bool { return !a.HasProvider(providers) })
	}

	if include, exclude := cfg.GlobalFilter(a.rt); len(include) > 0 || len(exclude) > 0 {
		a.rt.AllTools = a.rt.AllTools.Filtered(include, exclude)
	}
	return nil
}

// rebuildSlash updates execution, help and UI hints from the same active command set.
func (a *Assistant) rebuildSlash() {
	if a.slashRegistry == nil {
		return
	}
	newRegistry := slash.New(commands.All...)

	for _, cmd := range slash.FromCustomCommands(a.cfg.Commands) {
		if _, exists := newRegistry.Lookup(cmd.Name); exists {
			continue
		}

		newRegistry.Register(cmd)
	}

	// Register plugin commands (lowest priority: built-in > custom > plugin).
	for _, cmd := range a.tools.plugins.Commands() {
		if _, exists := newRegistry.Lookup(cmd.Name); exists {
			continue
		}

		newRegistry.Register(cmd)
	}

	*a.slashRegistry = *newRegistry
}

// Reload re-reads config from disk, rebuilds all derived state, and
// reconnects MCP servers. Conversation history, session state,
// stats, and runtime toggles are preserved.
func (a *Assistant) Reload(ctx context.Context) error {
	// Single config read for everything.
	newCfg, _, err := config.New(a.configOpts, config.AllParts()...)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	// Rebuild plugin cache (interpreters hold state, so only rebuild on explicit /reload).
	if a.rt.WithPlugins {
		effective, err := a.effectiveFeatures(newCfg, newCfg.Features)
		if err != nil {
			return err
		}
		newPluginCache, err := plugins.LoadAll(newCfg.Plugins, effective.PluginConfig, a.paths.Home)
		if err != nil {
			return fmt.Errorf("loading plugins: %w", err)
		}

		a.tools.plugins.Close()

		a.tools.plugins = newPluginCache
	}

	// Rebuild LSP manager.
	if a.tools.lsp != nil {
		a.tools.lsp.StopAll()
	}

	if len(newCfg.LSPServers) > 0 {
		a.tools.lsp = lsp.NewManager(
			lsp.Config{Servers: map[string]lsp.Server(newCfg.LSPServers)},
			a.configOpts.WorkDir,
		)
	} else {
		a.tools.lsp = nil
	}

	// Reload config and rebuild tools (uses the new pluginCache and LSP manager).
	if err := a.reloadConfig(&newCfg); err != nil {
		return err
	}

	// Clear model list cache — config may have changed providers/tokens/URLs.
	a.modelListCache = nil

	a.rebuildInjectors()

	// Close old MCP sessions and reconnect from new config.
	a.closeMCPSessions()
	a.reconnectMCP(ctx)

	return nil
}

// reconnectMCP closes old MCP sessions and reconnects from the current config.
// Synchronous — the user asked for reload, they can wait.
func (a *Assistant) reconnectMCP(ctx context.Context) {
	servers := make(map[string]mcp.Server)

	mcpInclude, mcpExclude := a.cfg.MCPFilter(a.rt)
	mcps := config.FilterMCPs(a.cfg.MCPs, mcpInclude, mcpExclude)

	for _, name := range mcps.Names() {
		if s := mcps[name]; s.IsEnabled() {
			servers[name] = s
		}
	}

	if len(servers) == 0 {
		return
	}

	a.send(ui.SpinnerMessage{Text: "Reconnecting MCP servers..."})

	results := mcp.ConnectAll(ctx, servers, func(name string) {
		a.send(ui.SpinnerMessage{Text: fmt.Sprintf("Connecting to %s...", name)})
	})

	var sessions []*mcp.Session

	for _, r := range results {
		if r.Error != nil {
			a.send(ui.CommandResult{
				Command: "mcp",
				Error:   fmt.Errorf("%s", r.StatusDisplay()),
			})

			continue
		}

		sessions = append(sessions, r.Session)
	}

	a.RegisterMCPTools(sessions)

	// Rebuild state so new MCP tools appear in prompt and status immediately.
	// Unlike initial startup (where the work callback controls timing), /reload
	// is user-initiated and should take effect right away.
	if err := a.rebuildState(); err != nil {
		a.send(ui.CommandResult{Command: "mcp", Error: err})
	}

	a.send(ui.SpinnerMessage{}) // clear
}
