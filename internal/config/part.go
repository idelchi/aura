package config

import (
	"fmt"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Part identifies a loadable section of the configuration.
// Callers pass parts to New() to declare exactly what they need.
type Part string

const (
	PartModes         Part = "modes"
	PartAgents        Part = "agents"
	PartSystems       Part = "systems"
	PartAgentsMd      Part = "agents-md"
	PartProviders     Part = "providers"
	PartMCPs          Part = "mcps"
	PartFeatures      Part = "features"
	PartCommands      Part = "commands"
	PartSkills        Part = "skills"
	PartToolDefs      Part = "tool-defs"
	PartHooks         Part = "hooks"
	PartLSP           Part = "lsp"
	PartTasks         Part = "tasks"
	PartPlugins       Part = "plugins"
	PartApprovalRules Part = "approval-rules"
)

// AllParts returns all config parts in load order.
func AllParts() []Part {
	return []Part{
		PartModes, PartAgents, PartSystems, PartAgentsMd, PartProviders,
		PartMCPs, PartFeatures, PartCommands, PartSkills, PartToolDefs,
		PartHooks, PartLSP, PartTasks, PartPlugins, PartApprovalRules,
	}
}

// loader is a function that loads a single config section into cfg.
type loader func(cfg *Config, fs Files, opts Options) error

// loaders maps each Part to its loader function.
// Each loader writes directly into the Config struct and is self-contained —
// post-load setup (e.g. SetAuthDirs for providers) lives inside the loader.
var loaders = map[Part]loader{
	PartModes: func(cfg *Config, fs Files, _ Options) error {
		var err error

		cfg.Modes, err = loadModes(fs.Modes)

		return err
	},
	PartAgents: func(cfg *Config, fs Files, opts Options) error {
		var err error

		cfg.Agents, err = loadAgents(fs.Agents, opts.Homes)

		return err
	},
	PartSystems: func(cfg *Config, fs Files, _ Options) error {
		var err error

		cfg.Systems, err = loadSystems(fs.Prompts)

		return err
	},
	PartAgentsMd: func(cfg *Config, fs Files, opts Options) error {
		cfg.AgentsMd = AgentsMds{}

		return cfg.AgentsMd.Load(fs.AgentsMd, opts.GlobalHome)
	},
	PartProviders: func(cfg *Config, fs Files, opts Options) error {
		var err error

		cfg.Providers, err = loadProviders(fs.Providers)
		if err != nil {
			return err
		}

		var authDirs []string

		if opts.WriteHome != "" {
			authDirs = append(authDirs, folder.New(opts.WriteHome, "auth").Path())
		}

		if opts.GlobalHome != "" {
			authDirs = append(authDirs, folder.New(opts.GlobalHome, "auth").Path())
		}

		cfg.Providers.Apply(func(p *Provider) {
			p.AuthDirs = authDirs
		})

		return nil
	},
	PartMCPs: func(cfg *Config, fs Files, _ Options) error {
		var err error

		cfg.MCPs, err = loadMCPs(fs.MCP)

		return err
	},
	PartFeatures: func(cfg *Config, fs Files, _ Options) error {
		cfg.Features = Features{}

		return cfg.Features.Load(fs.Features)
	},
	PartCommands: func(cfg *Config, fs Files, _ Options) error {
		var err error

		cfg.Commands, err = loadCommands(fs.Commands)

		return err
	},
	PartSkills: func(cfg *Config, fs Files, _ Options) error {
		var err error

		cfg.Skills, err = loadSkills(fs.Skills)

		return err
	},
	PartToolDefs: func(cfg *Config, fs Files, _ Options) error {
		cfg.ToolDefs = ToolDefs{}

		return cfg.ToolDefs.Load(fs.Tools)
	},
	PartHooks: func(cfg *Config, fs Files, _ Options) error {
		cfg.Hooks = Hooks{}

		return cfg.Hooks.Load(fs.Hooks)
	},
	PartLSP: func(cfg *Config, fs Files, _ Options) error {
		cfg.LSPServers = LSPServers{}

		return cfg.LSPServers.Load(fs.LSP)
	},
	PartTasks: func(cfg *Config, fs Files, opts Options) error {
		// Merge extra task files before loading.
		// fs is by value — appends are local, consumed immediately.
		for _, pattern := range opts.ExtraTaskFiles {
			matches, err := doublestar.FilepathGlob(pattern)
			if err != nil {
				return fmt.Errorf("resolving task file glob %q: %w", pattern, err)
			}

			if len(matches) == 0 {
				return fmt.Errorf("task file pattern %q matched no files", pattern)
			}

			for _, match := range matches {
				fs.Tasks = append(fs.Tasks, file.New(match))
			}
		}

		cfg.Tasks = task.Tasks{}

		return cfg.Tasks.Load(fs.Tasks, opts.SetVars)
	},
	PartPlugins: func(cfg *Config, fs Files, _ Options) error {
		var err error

		cfg.Plugins, err = loadPlugins(fs.Plugins)

		return err
	},
	PartApprovalRules: func(cfg *Config, fs Files, opts Options) error {
		cfg.ApprovalRules = ApprovalRules{}

		return cfg.ApprovalRules.Load(fs.Rules, opts.GlobalHome)
	},
}
