package config

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/prompts"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
)

// Type identifies the scope of an AGENTS.md file.
type Type string

const (
	// Global represents a user-wide AGENTS.md configuration.
	Global Type = "global"
	// Local represents a project-specific AGENTS.md configuration.
	Local Type = "local"
)

// AgentsMdFilter controls which AGENTS.md scopes are injected into an agent's prompt.
type AgentsMdFilter string

const (
	// AgentsMdAll injects all AGENTS.md files (default when field is empty).
	AgentsMdAll AgentsMdFilter = ""
	// AgentsMdGlobal injects only global-scoped AGENTS.md files.
	AgentsMdGlobal AgentsMdFilter = "global"
	// AgentsMdLocal injects only local-scoped AGENTS.md files.
	AgentsMdLocal AgentsMdFilter = "local"
	// AgentsMdNone skips all AGENTS.md injection.
	AgentsMdNone AgentsMdFilter = "none"
)

// Includes returns whether the given AGENTS.md scope passes this filter.
func (f AgentsMdFilter) Includes(t Type) bool {
	switch f {
	case AgentsMdNone:
		return false
	case AgentsMdGlobal:
		return t == Global
	case AgentsMdLocal:
		return t == Local
	default: // "" or "all"
		return true
	}
}

// AgentsMd represents an AGENTS.md configuration file with its scope.
type AgentsMd struct {
	// Prompt is the content of the AGENTS.md file.
	Prompt prompts.Prompt
	// Type identifies whether this is a global or local configuration.
	Type Type
}

// AgentsMds maps files to their corresponding AgentsMd configurations.
type AgentsMds map[file.File]AgentsMd

// Load populates the AgentsMds map from the given files.
// globalHome is the path to ~/.aura (or "") — files under it are tagged Global, others Local.
func (a *AgentsMds) Load(files files.Files, globalHome string) error {
	for _, file := range files {
		var agentsMd AgentsMd

		prompt, err := file.ReadString()
		if err != nil {
			return fmt.Errorf("reading %s: %w", file, err)
		}

		agentsMd.Prompt = prompts.Prompt(prompt)

		agentsMd.Type = Local

		if globalHome != "" && strings.HasPrefix(file.Path(), globalHome) {
			agentsMd.Type = Global
		}

		(*a)[file] = agentsMd
	}

	return nil
}
