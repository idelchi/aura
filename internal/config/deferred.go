package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// DeferredToolIndex groups deferred tools by source for system prompt rendering.
type DeferredToolIndex struct {
	BuiltIn []string            // non-MCP tool names
	Servers map[string][]string // server name → short tool names
}

// NewDeferredToolIndex classifies deferred tools into built-in and per-server groups.
func NewDeferredToolIndex(tools tool.Tools) DeferredToolIndex {
	idx := DeferredToolIndex{
		Servers: make(map[string][]string),
	}

	for _, t := range tools {
		if server, short, ok := mcp.ExtractServer(t.Name()); ok {
			idx.Servers[server] = append(idx.Servers[server], short)
		} else {
			idx.BuiltIn = append(idx.BuiltIn, t.Name())
		}
	}

	return idx
}

// Render returns a system prompt block listing all deferred tools.
// Returns empty string if the index is empty.
func (d DeferredToolIndex) Render() string {
	if len(d.BuiltIn) == 0 && len(d.Servers) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("The following tools are available but not yet loaded. ")
	b.WriteString("Use LoadTools to load them before use:\n")

	for _, name := range d.BuiltIn {
		fmt.Fprintf(&b, "- %s\n", name)
	}

	for _, server := range slices.Sorted(maps.Keys(d.Servers)) {
		names := d.Servers[server]
		for _, name := range names {
			fmt.Fprintf(&b, "- mcp__%s__%s\n", server, name)
		}
	}

	return b.String()
}
