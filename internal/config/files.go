package config

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/glob"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/files"
	"github.com/idelchi/godyl/pkg/path/folder"

	"go.yaml.in/yaml/v4"
)

// Files holds the different configuration files used by Aura.
type Files struct {
	Providers files.Files
	Modes     files.Files
	Agents    files.Files
	Prompts   files.Files
	AgentsMd  files.Files
	MCP       files.Files
	Features  files.Files
	Commands  files.Files
	Skills    files.Files
	Tools     files.Files
	Hooks     files.Files
	Tasks     files.Files
	LSP       files.Files
	Plugins   files.Files
	Rules     files.Files
}

// excludeExamples filters out .example. files (e.g. agent.example.md, provider.example.yaml).
// These files serve as documented references and must not be loaded as real config.
func excludeExamples(ff files.Files) files.Files {
	var result files.Files

	for _, f := range ff {
		if !strings.Contains(f.Base(), ".example.") {
			result = append(result, f)
		}
	}

	return result
}

// ResolvePluginDir returns the plugin directory for a given config home.
// It peeks at {home}/config/features/plugins.yaml for a "dir" field.
// Falls back to {home}/plugins when unset or unreadable.
func ResolvePluginDir(home string) string {
	f := folder.New(home, "config", "features").WithFile("plugins.yaml")

	data, err := f.Read()
	if err != nil {
		return folder.New(home, "plugins").Path()
	}

	var raw struct {
		Plugins struct {
			Dir string `yaml:"dir"`
		} `yaml:"plugins"`
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		debug.Log("[config] plugins.yaml exists but is invalid YAML: %v", err)

		return folder.New(home, "plugins").Path()
	}

	if raw.Plugins.Dir == "" {
		return folder.New(home, "plugins").Path()
	}

	d := folder.New(raw.Plugins.Dir)
	if !d.IsAbs() {
		d = folder.New(home, raw.Plugins.Dir)
	}

	return d.Path()
}

// DiscoverPlugins walks root recursively, collecting plugin.yaml files.
// When a plugin.yaml is found, its directory is not descended further (SkipDir),
// which prevents scanning vendor/ or nested plugin trees.
// Hidden directories (. prefix) and vendor/ directories are skipped entirely.
func DiscoverPlugins(root folder.Folder) (files.Files, error) {
	var result files.Files

	err := root.Walk(func(path file.File, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || (name != "." && strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}

			return nil
		}

		if d.Name() == "plugin.yaml" {
			result = append(result, path)

			return filepath.SkipDir
		}

		return nil
	})

	return result, err
}

// Load discovers and loads all configuration files from the specified directories.
// Multiple homes are processed in order (e.g. global then project); later entries override earlier ones
// via name-based dedup in each type's Load method.
// Files with ".example." in their basename are excluded — they serve as reference documentation only.
func (fs *Files) Load(cwd, launchDir string, homes ...string) error {
	seen := make(map[string]bool, len(homes))

	for i, home := range homes {
		if home == "" || seen[home] {
			continue
		}

		seen[home] = true

		debug.Log("[config]   home %d/%d: %s", i+1, len(homes), home)

		config := folder.New(home, "config")

		if dir := config.Join("providers"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering providers config: %w", err)
			} else {
				fs.Providers = append(fs.Providers, excludeExamples(files)...)
				debug.Log("[config]     providers: %d files", len(files))
			}
		}

		if dir := config.Join("prompts"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.md"); err != nil {
				return fmt.Errorf("discovering prompts config: %w", err)
			} else {
				fs.Prompts = append(fs.Prompts, excludeExamples(files)...)
				debug.Log("[config]     prompts: %d files", len(files))
			}
		}

		if dir := config.Join("modes"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.md"); err != nil {
				return fmt.Errorf("discovering modes config: %w", err)
			} else {
				fs.Modes = append(fs.Modes, excludeExamples(files)...)
				debug.Log("[config]     modes: %d files", len(files))
			}
		}

		if dir := config.Join("agents"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.md"); err != nil {
				return fmt.Errorf("discovering agents config: %w", err)
			} else {
				fs.Agents = append(fs.Agents, excludeExamples(files)...)
				debug.Log("[config]     agents: %d files", len(files))
			}
		}

		if dir := config.Join("sandbox"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering sandbox config: %w", err)
			} else {
				fs.Features = append(fs.Features, excludeExamples(files)...)
				debug.Log("[config]     sandbox: %d files", len(files))
			}
		}

		if dir := config.Join("mcp"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering mcp config: %w", err)
			} else {
				fs.MCP = append(fs.MCP, excludeExamples(files)...)
				debug.Log("[config]     mcp: %d files", len(files))
			}
		}

		if dir := config.Join("features"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering features config: %w", err)
			} else {
				fs.Features = append(fs.Features, excludeExamples(files)...)
				debug.Log("[config]     features: %d files", len(files))
			}
		}

		if cmdsDir := config.Join("commands"); cmdsDir.Exists() {
			if files, err := glob.Glob(cmdsDir, "**/*.md"); err != nil {
				return fmt.Errorf("discovering commands config: %w", err)
			} else {
				fs.Commands = append(fs.Commands, excludeExamples(files)...)
				debug.Log("[config]     commands: %d files", len(files))
			}
		}

		if skillsDir := folder.New(home, "skills"); skillsDir.Exists() {
			if files, err := glob.Glob(skillsDir, "**/*.md"); err != nil {
				return fmt.Errorf("discovering skills config: %w", err)
			} else {
				fs.Skills = append(fs.Skills, excludeExamples(files)...)
				debug.Log("[config]     skills: %d files", len(files))
			}
		}

		if toolsDir := config.Join("tools"); toolsDir.Exists() {
			if files, err := glob.Glob(toolsDir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering tools config: %w", err)
			} else {
				fs.Tools = append(fs.Tools, excludeExamples(files)...)
				debug.Log("[config]     tools: %d files", len(files))
			}
		}

		if dir := config.Join("hooks"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering hooks config: %w", err)
			} else {
				fs.Hooks = append(fs.Hooks, excludeExamples(files)...)
				debug.Log("[config]     hooks: %d files", len(files))
			}
		}

		if dir := config.Join("lsp"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering lsp config: %w", err)
			} else {
				fs.LSP = append(fs.LSP, excludeExamples(files)...)
				debug.Log("[config]     lsp: %d files", len(files))
			}
		}

		if tasksDir := config.Join("tasks"); tasksDir.Exists() {
			if files, err := glob.Glob(tasksDir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering tasks config: %w", err)
			} else {
				fs.Tasks = append(fs.Tasks, excludeExamples(files)...)
				debug.Log("[config]     tasks: %d files", len(files))
			}
		}

		if dir := config.Join("rules"); dir.Exists() {
			if files, err := glob.Glob(dir, "**/*.yaml"); err != nil {
				return fmt.Errorf("discovering rules config: %w", err)
			} else {
				fs.Rules = append(fs.Rules, excludeExamples(files)...)
				debug.Log("[config]     rules: %d files", len(files))
			}
		}

		pluginDir := ResolvePluginDir(home)
		debug.Log("[config]     plugin dir: %s", pluginDir)

		if dir := folder.New(pluginDir); dir.Exists() {
			debug.Log("[config]     discovering plugins...")

			if found, err := DiscoverPlugins(dir); err != nil {
				return fmt.Errorf("discovering plugins: %w", err)
			} else {
				fs.Plugins = append(fs.Plugins, excludeExamples(found)...)
				debug.Log("[config]     plugins: %d found", len(found))
			}
		}
	}

	// AGENTS.md: collect from each config home, then walk upward from WorkDir.
	// Walk stops at .git, LaunchDir, or filesystem root — whichever comes first.
	// Without --workdir (WorkDir == LaunchDir), produces exactly one candidate.
	debug.Log("[config]   resolving AGENTS.md...")

	var agentsMd files.Files

	for _, home := range homes {
		if home != "" {
			agentsMd = append(agentsMd, folder.New(home).Expanded().WithFile("AGENTS.md"))
		}
	}

	// Walk upward from WorkDir toward LaunchDir.
	dir := folder.New(cwd)
	launchAbs, _ := filepath.Abs(launchDir)

	for {
		agentsMd = append(agentsMd, dir.WithFile("AGENTS.md"))

		// Stop: git root found at this level.
		if folder.New(dir.Path(), ".git").Exists() {
			break
		}

		// Stop: reached the directory where aura was launched.
		dirAbs, _ := filepath.Abs(dir.Path())
		if dirAbs == launchAbs {
			break
		}

		// Stop: filesystem root (can't go higher).
		parent := dir.Dir()
		if parent.Path() == dir.Path() {
			break
		}

		dir = parent
	}

	debug.Log("[config]   AGENTS.md candidates: %d", len(agentsMd))

	agentsMd.Existing()

	// Dedup by absolute path — config homes and walk may produce the same file.
	seenPaths := make(map[string]bool)

	var deduped files.Files

	for _, f := range agentsMd {
		abs, _ := filepath.Abs(f.Path())
		if !seenPaths[abs] {
			seenPaths[abs] = true

			deduped = append(deduped, f)
		}
	}

	agentsMd = deduped

	debug.Log("[config]   AGENTS.md found: %d", len(agentsMd))

	fs.AgentsMd = agentsMd

	return nil
}
