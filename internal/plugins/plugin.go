package plugins

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/cogentcore/yaegi/interp"
	"github.com/cogentcore/yaegi/stdlib"
	"github.com/cogentcore/yaegi/stdlib/unrestricted"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/sdk"
	"github.com/idelchi/aura/sdk/version"
	"github.com/idelchi/godyl/pkg/env"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Plugin holds a loaded Yaegi interpreter and its discovered hooks and/or tool.
type Plugin struct {
	name      string
	dir       string
	basePkg   string
	condition string
	enabled   bool
	once      bool
	configDir string // project .aura/ directory, passed to tool Init()
	interp    *interp.Interpreter
	hooks     []*Hook
	tool      *PluginTool
	command   *PluginCommand

	mergedConfig map[string]any // merged plugin config (plugin.yaml → global → local)

	// Environment injection for task-scoped env vars.
	envWhitelist []string                   // from config.Plugin.Env
	unrestricted bool                       // env: ["*"]
	setenv       func(string, string) error // extracted from Yaegi's os.Setenv
}

// Load creates a Plugin, initializes the Yaegi interpreter with GOPATH-based
// package loading, and probes for hook and tool functions. The goPath parameter is the
// shared temp GOPATH owned by Cache. When unsafe is true, os/exec and other
// restricted imports are available to the plugin. configDir is the project .aura/ directory.
func Load(
	name string,
	cfg config.Plugin,
	goPath string,
	unsafe bool,
	configDir string,
	mergedConfig map[string]any,
) (*Plugin, error) {
	dir := cfg.Dir()

	// Read module path from go.mod.
	modulePath, err := readModulePath(dir)
	if err != nil {
		return nil, fmt.Errorf("plugin %q: %w", name, err)
	}

	// Derive Go package name from module path.
	basePkg := strings.ReplaceAll(path.Base(modulePath), "-", "_")

	// Copy plugin source + vendor/ into GoPath preserving structure.
	if err := copyPluginToGoPath(dir, folder.New(goPath, "src", modulePath).Path()); err != nil {
		return nil, fmt.Errorf("plugin %q: copying source: %w", name, err)
	}

	// Copy vendored sdk/version into top-level GOPATH so Yaegi can import it.
	if err := copySDKVersionToGoPath(dir, goPath); err != nil {
		return nil, fmt.Errorf("plugin %q: copying sdk/version: %w", name, err)
	}

	opts := interp.Options{GoPath: goPath}

	if slices.Contains(cfg.Env, "*") {
		opts.Unrestricted = true
	} else if len(cfg.Env) > 0 {
		processEnv := env.FromEnv()
		filtered := processEnv.GetWithPredicates(func(k, _ string) bool {
			return slices.Contains(cfg.Env, k)
		})

		opts.Env = filtered.AsSlice()
	}

	// Yaegi interpreter — Go 1.25 stdlib via cogentcore/yaegi fork.
	// Constraints: no generics, no min/max builtins, no range-over-func,
	// no host-side reflection on interpreted types. See docs/features/plugins.md#limitations.
	i := interp.New(opts)
	i.Use(stdlib.Symbols)

	if unsafe {
		i.Use(unrestricted.Symbols)
	}

	i.Use(sdk.Symbols)

	// Import via module path — Yaegi's GTA parses all files, resolves vendored deps.
	if _, err := i.Eval(fmt.Sprintf(`import %q`, modulePath)); err != nil {
		if !unsafe && isRestrictedImportError(err) {
			return nil, fmt.Errorf(
				"plugin %q requires unsafe mode (os/exec) — set plugins.unsafe: true in features/plugins.yaml or pass --unsafe-plugins: %w",
				name,
				err,
			)
		}

		return nil, fmt.Errorf("plugin %q: loading %s: %w\n\nTry: aura plugins update %s", name, dir, err, name)
	}

	// Check SDK compatibility after interpreter is ready — Yaegi reads the version directly.
	pluginVersion := probeSDKVersion(i)

	if err := IsSDKCompatible(pluginVersion, version.Version); err != nil {
		return nil, fmt.Errorf("plugin %q: %w", name, err)
	}

	// Extract os.Setenv for runtime env injection (task env vars).
	// Sandboxed plugins get Yaegi's sandboxed version (updates interp.env map).
	// Unrestricted plugins get the real os.Setenv.
	var setenvFn func(string, string) error

	if _, err := i.Eval(`import "os"`); err == nil {
		if v, err := i.Eval("os.Setenv"); err == nil {
			if fn, ok := v.Interface().(func(string, string) error); ok {
				setenvFn = fn
			}
		}
	}

	p := &Plugin{
		name:         name,
		dir:          dir,
		basePkg:      basePkg,
		condition:    cfg.Condition,
		enabled:      cfg.IsEnabled(),
		once:         cfg.Once,
		configDir:    configDir,
		interp:       i,
		mergedConfig: mergedConfig,
		envWhitelist: cfg.Env,
		unrestricted: slices.Contains(cfg.Env, "*"),
		setenv:       setenvFn,
	}

	if err := p.probeHooks(basePkg); err != nil {
		return nil, err
	}

	if err := p.probeTool(cfg); err != nil {
		return nil, err
	}

	if err := p.probeCommand(); err != nil {
		return nil, err
	}

	if len(p.hooks) == 0 && p.tool == nil && p.command == nil {
		return nil, fmt.Errorf(
			"plugin %q: no hook, tool, or command functions found "+
				"(expected BeforeChat/.../OnError, Schema/Execute, or Command/ExecuteCommand in package %q)",
			name, basePkg,
		)
	}

	toolInfo := ""

	if p.tool != nil {
		toolInfo = ", tool=" + p.tool.Name()
	}

	cmdInfo := ""

	if p.command != nil {
		cmdInfo = ", command=" + p.command.Name()
	}

	debug.Log("[plugin] loaded %q from %s (%d hooks%s%s, pkg=%s)", name, dir, len(p.hooks), toolInfo, cmdInfo, basePkg)

	return p, nil
}

// probeHooks looks up each known hook symbol in the interpreter.
// Missing hooks are silently skipped; type mismatches are errors.
func (p *Plugin) probeHooks(basePkg string) error {
	for _, def := range KnownTimingNames() {
		symbol := fmt.Sprintf("%s.%s", basePkg, def.Name)

		v, err := p.interp.Eval(symbol)
		if err != nil {
			continue
		}

		hook, err := newHook(p.name, def.Timing, p.condition, p.enabled, p.once, v)
		if err != nil {
			return fmt.Errorf("plugin %q: %w", p.name, err)
		}

		hook.injectEnv = p.InjectEnv
		hook.pluginConfig = p.mergedConfig
		p.hooks = append(p.hooks, hook)
		debug.Log("[plugin] %q: registered %s hook", p.name, def.Timing)
	}

	return nil
}

// Hooks returns the injector wrappers for all discovered hooks.
func (p *Plugin) Hooks() []*Hook {
	return p.hooks
}

// Tool returns the tool exported by this plugin, or nil if it's hook-only.
func (p *Plugin) Tool() *PluginTool {
	return p.tool
}

// Command returns the command exported by this plugin, or nil if none.
func (p *Plugin) Command() *PluginCommand {
	return p.command
}

// probeCommand looks for Command and ExecuteCommand exports in the plugin interpreter.
// If Command is found, ExecuteCommand is required.
func (p *Plugin) probeCommand() error {
	schemaVal, err := p.interp.Eval(p.basePkg + ".Command")
	if err != nil {
		return nil // no command export
	}

	schemaFn, ok := schemaVal.Interface().(func() sdk.CommandSchema)
	if !ok {
		return fmt.Errorf("plugin %q: Command has wrong signature, expected func() sdk.CommandSchema", p.name)
	}

	execVal, err := p.interp.Eval(p.basePkg + ".ExecuteCommand")
	if err != nil {
		return fmt.Errorf("plugin %q: exports Command but no ExecuteCommand function", p.name)
	}

	execFn, ok := execVal.Interface().(func(context.Context, string, sdk.Context) (sdk.CommandResult, error))
	if !ok {
		return fmt.Errorf(
			"plugin %q: ExecuteCommand has wrong signature, expected func(context.Context, string, sdk.Context) (sdk.CommandResult, error)",
			p.name,
		)
	}

	schema := schemaFn()

	p.command = &PluginCommand{
		schema:       schema,
		execute:      execFn,
		injectEnv:    p.InjectEnv,
		pluginConfig: p.mergedConfig,
	}

	debug.Log("[plugin] %q: registered command /%s", p.name, schema.Name)

	return nil
}

// Close releases resources. Currently a no-op (Yaegi has no explicit close).
func (p *Plugin) Close() {}

// InjectEnv injects task-scoped env vars into the Yaegi interpreter.
// Only vars in the plugin's whitelist are injected. Unrestricted plugins get all vars.
func (p *Plugin) InjectEnv(e env.Env) {
	if p.setenv == nil || len(e) == 0 {
		return
	}

	for k, v := range e {
		if p.unrestricted || slices.Contains(p.envWhitelist, k) {
			p.setenv(k, v)
		}
	}
}

// isRestrictedImportError checks if the error is caused by a restricted import (e.g. os/exec).
func isRestrictedImportError(err error) bool {
	msg := err.Error()

	return strings.Contains(msg, "os/exec") ||
		strings.Contains(msg, `unable to find source related to: "os/exec"`)
}
