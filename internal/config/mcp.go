package config

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/mcp"
	"github.com/idelchi/aura/pkg/wildcard"
	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/files"
)

// FilterMCPs returns a subset of servers matching the include/exclude glob patterns.
// Exclude takes precedence over include. Empty include = all pass.
func FilterMCPs(mcps StringCollection[mcp.Server], include, exclude []string) StringCollection[mcp.Server] {
	if len(include) == 0 && len(exclude) == 0 {
		return mcps
	}

	return mcps.Filter(func(name string, _ mcp.Server) bool {
		if wildcard.MatchAny(name, exclude...) {
			return false
		}

		return len(include) == 0 || wildcard.MatchAny(name, include...)
	})
}

// loadMCPs populates a StringCollection of MCP servers from the given files.
func loadMCPs(ff files.Files) (StringCollection[mcp.Server], error) {
	result := make(StringCollection[mcp.Server])

	for _, f := range ff {
		content, err := f.Read()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}

		var temp map[string]mcp.Server
		if err := yamlutil.StrictUnmarshal(content, &temp); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", f, err)
		}

		for key, val := range temp {
			val.Source = f.Path()
			result[strings.ToLower(key)] = val
		}
	}

	return result, nil
}
