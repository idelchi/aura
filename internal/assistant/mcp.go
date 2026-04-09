package assistant

import (
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/mcp"
)

// closeMCPSessions removes MCP tools from AllTools, closes all sessions, and clears the slice.
// Must be called before reconnecting to avoid stale tools blocking fresh ones
// (Tools.Add skips tools that already exist by name).
func (a *Assistant) closeMCPSessions() {
	for _, s := range a.tools.mcp {
		for _, t := range s.Tools() {
			a.rt.AllTools.Remove(t.Name())
		}

		if err := s.Close(); err != nil {
			debug.Log("[mcp] close %s: %v", s.Name, err)
		}
	}

	a.tools.mcp = nil
}

// addMCPTools adds tools from sessions and re-applies global filters.
func (a *Assistant) addMCPTools(sessions []*mcp.Session) {
	for _, s := range sessions {
		for _, t := range s.Tools() {
			a.rt.AllTools.Add(t)
		}
	}

	if include, exclude := a.cfg.GlobalFilter(a.rt); len(include) > 0 || len(exclude) > 0 {
		a.rt.AllTools = a.rt.AllTools.Filtered(include, exclude)
	}
}

// RegisterMCPTools integrates MCP sessions into the running assistant.
// It adds discovered tools to AllTools and re-applies CLI tool filters.
// It does NOT call rebuildState — the caller (RunSession) is responsible
// for triggering a rebuild after all MCP servers are connected.
func (a *Assistant) RegisterMCPTools(sessions []*mcp.Session) {
	a.tools.mcp = sessions
	a.addMCPTools(sessions)
}

// RegisterMCPSession appends a single MCP session mid-conversation.
// Unlike RegisterMCPTools (used at startup), this calls rebuildState so
// the new tools appear immediately in the agent's tool list and status bar.
func (a *Assistant) RegisterMCPSession(s *mcp.Session) error {
	a.tools.mcp = append(a.tools.mcp, s)
	a.addMCPTools([]*mcp.Session{s})

	return a.rebuildState()
}
