package config

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/lsp"
	"github.com/idelchi/aura/pkg/yamlutil"
	"github.com/idelchi/godyl/pkg/path/files"
)

// LSPServers maps server names to their LSP server configurations.
// Loaded from .aura/config/lsp/**/*.yaml.
type LSPServers map[string]lsp.Server

// Load reads YAML files and merges all LSP server definitions into the map.
// Each file contains a map of server names to their definitions.
// Multiple servers can live in one file, or each server in its own file —
// the loader is decoupled from filenames.
func (s *LSPServers) Load(ff files.Files) error {
	*s = make(LSPServers)

	for _, f := range ff {
		content, err := f.Read()
		if err != nil {
			return fmt.Errorf("reading LSP server definition %s: %w", f, err)
		}

		var file map[string]lsp.Server
		if err := yamlutil.StrictUnmarshal(content, &file); err != nil {
			return fmt.Errorf("parsing LSP server definition %s: %w", f, err)
		}

		for name, server := range file {
			(*s)[strings.ToLower(name)] = server
		}
	}

	return nil
}
