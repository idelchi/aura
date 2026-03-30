package assistant

import (
	"fmt"
	"slices"
	"strings"

	"github.com/idelchi/aura/internal/agent"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/conversation"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/internal/session"
	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/snapshot"
	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/internal/todo"
	"github.com/idelchi/aura/internal/ui"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/providers"
)

// Compile-time check that *Assistant implements slash.Context.
var _ slash.Context = (*Assistant)(nil)

// Resolved returns the effective runtime configuration snapshot.
func (a *Assistant) Resolved() config.Resolved { return a.resolved.config }

// SystemPrompt returns the current rendered system prompt.
func (a *Assistant) SystemPrompt() string { return a.agent.Prompt }

// ToolNames returns the names of the currently active tools.
func (a *Assistant) ToolNames() []string { return a.agent.Tools.Names() }

// LoadedTools returns the names of deferred tools loaded this session.
func (a *Assistant) LoadedTools() []string { return loadedToolNames(a.tools.loaded) }

// loadedToolNames converts a loaded-tools map to a sorted slice.
func loadedToolNames(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}

	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}

	slices.Sort(names)

	return names
}

// mcpServerNames extracts server names from MCP sessions.
func mcpServerNames(sessions []*mcp.Session) []string {
	if len(sessions) == 0 {
		return nil
	}

	names := make([]string, len(sessions))
	for i, s := range sessions {
		names[i] = s.Name
	}

	return names
}

// ResolvedModel returns the resolved model metadata.
func (a *Assistant) ResolvedModel() model.Model { return a.resolved.model.Deref() }

// ToolPolicy returns the effective tool policy (includes persisted approval rules).
func (a *Assistant) ToolPolicy() *config.ToolPolicy { return a.resolved.toolPolicy }

// Cfg returns the runtime configuration.
func (a *Assistant) Cfg() config.Config { return a.cfg }

// Builder returns the conversation builder.
func (a *Assistant) Builder() *conversation.Builder { return a.builder }

// SessionManager returns the session persistence manager.
func (a *Assistant) SessionManager() *session.Manager { return a.session.manager }

// TodoList returns the per-session todo list.
func (a *Assistant) TodoList() *todo.List { return a.tools.todo }

// SessionStats returns the session metrics tracker.
func (a *Assistant) SessionStats() *stats.Stats { return a.session.stats }

// InjectorRegistry returns the synthetic message injection registry.
func (a *Assistant) InjectorRegistry() *injector.Registry { return a.tools.injectors }

// PluginSummary returns a formatted display of all loaded plugins and their capabilities.
func (a *Assistant) PluginSummary() string {
	var b strings.Builder

	// Hooks (from injector registry — existing display).
	injDisplay := a.tools.injectors.Display()
	if injDisplay != "" {
		b.WriteString(injDisplay)
	}

	// Tools.
	tools := a.tools.plugins.Tools()
	if len(tools) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}

		b.WriteString("Plugin Tools:\n")

		for _, t := range tools {
			fmt.Fprintf(&b, "  %s\n", t.Name())
		}
	}

	// Commands.
	cmds := a.tools.plugins.Commands()
	if len(cmds) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}

		b.WriteString("Plugin Commands:\n")

		for _, cmd := range cmds {
			fmt.Fprintf(&b, "  %s  %s\n", cmd.Name, cmd.Description)
		}
	}

	if b.Len() == 0 {
		return "No plugins loaded."
	}

	return strings.TrimRight(b.String(), "\n")
}

// MCPSessions returns the connected MCP server sessions.
func (a *Assistant) MCPSessions() []*mcp.Session { return a.tools.mcp }

// SnapshotManager returns the snapshot manager (nil if not in a git repo).
func (a *Assistant) SnapshotManager() *snapshot.Manager { return a.tools.snapshots }

// EventChan returns the UI event channel for sending events.
func (a *Assistant) EventChan() chan<- ui.Event { return a.events }

// SetVerbose sets the UI thinking display preference.
func (a *Assistant) SetVerbose(val bool) {
	a.toggles.verbose = val
	a.resolved.config.Verbose = val
	a.EmitDisplayHints()
}

// ReadBeforePolicy returns the current read-before enforcement policy.
func (a *Assistant) ReadBeforePolicy() tool.ReadBeforePolicy { return a.tracker.Policy() }

// SetReadBeforePolicy sets the read-before enforcement policy (records as runtime override)
// and rebuilds state so the system prompt reflects the change.
func (a *Assistant) SetReadBeforePolicy(p tool.ReadBeforePolicy) error {
	a.tracker.SetPolicy(p)

	return a.rebuildState()
}

// RequestExit signals the assistant loop to exit gracefully.
func (a *Assistant) RequestExit() { a.toggles.exitRequested = true }

// ModelListCache returns the cached model list (nil on first call).
func (a *Assistant) ModelListCache() []slash.ProviderModels { return a.modelListCache }

// CacheModelList stores the model list cache.
func (a *Assistant) CacheModelList(v []slash.ProviderModels) { a.modelListCache = v }

// ClearModelListCache invalidates the cached model list.
func (a *Assistant) ClearModelListCache() { a.modelListCache = nil }

// TemplateVars returns the session-wide --set template variables.
func (a *Assistant) TemplateVars() map[string]string { return a.setVars }

// SetNoop enables noop mode — all agent providers are replaced with the noop provider.
// Future agent rebuilds (SetAgent, Reload) and on-demand feature agents (via ResolveAgent)
// also use this provider.
func (a *Assistant) SetNoop(p providers.Provider) {
	a.noopProvider = p
	a.agent.Provider = p
}

// applyNoop replaces the given agent's provider with the noop provider if noop mode is active.
func (a *Assistant) applyNoop(ag *agent.Agent) {
	if a.noopProvider != nil && ag != nil {
		ag.Provider = a.noopProvider
	}
}
