package plugins

import (
	"context"
	"fmt"
	"os"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/internal/tools/filetime"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/sdk"
	"github.com/idelchi/godyl/pkg/env"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// PluginTool wraps a Yaegi-interpreted plugin's tool exports as a compiled tool.Tool.
// Bridges plugin-declared Schema/Execute/Paths to the host's sandbox and filetime infrastructure.
type PluginTool struct {
	tool.Base

	name         string
	override     bool
	optIn        bool
	available    bool
	sandboxable  bool
	parallel     bool
	params       tool.Parameters
	pluginConfig map[string]any

	execute   func(context.Context, sdk.Context, map[string]any) (string, error)
	paths     func(map[string]any) (sdk.ToolPaths, error)
	injectEnv func(env.Env)
}

func (t *PluginTool) Name() string { return t.name }

// Schema composes the full schema dynamically so that MergeText overrides are visible.
func (t *PluginTool) Schema() tool.Schema {
	return tool.Schema{
		Name:        t.name,
		Description: tool.ComposeDescription(t),
		Parameters:  t.params,
	}
}

// Sandboxable returns whether the tool supports sandbox re-exec.
// Default is true. Plugins can override by exporting Sandboxable() bool.
// Implements tool.SandboxOverride.
func (t *PluginTool) Sandboxable() bool { return t.sandboxable }

// Parallel returns whether the tool is safe to run concurrently with other tools.
// Default is true. Plugins can override by exporting Parallel() bool.
// Implements tool.ParallelOverride.
func (t *PluginTool) Parallel() bool  { return t.parallel }
func (t *PluginTool) Available() bool { return t.available }
func (t *PluginTool) Overrides() bool { return t.override }
func (t *PluginTool) IsOptIn() bool   { return t.optIn }

func (t *PluginTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if err := t.Schema().ValidateArgs(args); err != nil {
		return "", err
	}

	if t.injectEnv != nil {
		t.injectEnv(task.EnvFromContext(ctx))
	}

	// Build sdk.Context from the Go context (injected by assistant).
	var sdkCtx sdk.Context

	if sc := tool.SDKContextFromContext(ctx); sc != nil {
		sdkCtx = sc.(sdk.Context)
	}

	sdkCtx.PluginConfig = t.pluginConfig

	if sdkCtx.Workdir == "" {
		sdkCtx.Workdir = tool.WorkDirFromContext(ctx)
	}

	return t.execute(ctx, sdkCtx, args)
}

// Paths returns read/write paths for the sandbox fast-path.
func (t *PluginTool) Paths(ctx context.Context, args map[string]any) (read, write []string, err error) {
	if t.paths == nil {
		return nil, nil, nil
	}

	tp, err := t.paths(args)
	if err != nil {
		return nil, nil, err
	}

	for i, p := range tp.Read {
		tp.Read[i] = tool.ResolvePath(ctx, p)
	}

	for i, p := range tp.Write {
		tp.Write[i] = tool.ResolvePath(ctx, p)
	}

	return tp.Read, tp.Write, nil
}

// Pre enforces read-before-write via filetime using the Guard list.
// All Guard paths are write-type operations, so enforcement is gated on policy.Write.
func (t *PluginTool) Pre(ctx context.Context, args map[string]any) error {
	if t.paths == nil {
		return nil
	}

	tracker := filetime.FromContext(ctx)
	policy := tracker.Policy()

	if !policy.Write {
		return nil
	}

	tp, err := t.paths(args)
	if err != nil {
		return err
	}

	for _, p := range tp.Guard {
		resolved := tool.ResolvePath(ctx, p)
		if !file.New(resolved).Exists() {
			continue // new file, no read required
		}

		if err := tracker.AssertRead(resolved); err != nil {
			return err
		}
	}

	return nil
}

// Post records filetime reads using the Record list and clears using the Clear list.
func (t *PluginTool) Post(ctx context.Context, args map[string]any) {
	if t.paths == nil {
		return
	}

	tp, err := t.paths(args)
	if err != nil {
		debug.Log("[plugin] post paths error for %s: %v", t.name, err)

		return
	}

	tracker := filetime.FromContext(ctx)

	for _, p := range tp.Record {
		tracker.RecordRead(tool.ResolvePath(ctx, p))
	}

	for _, p := range tp.Clear {
		tracker.ClearRead(tool.ResolvePath(ctx, p))
	}
}

// probeTool looks for Schema and Execute exports in the plugin interpreter.
// If Schema is found, Execute is required. Paths is optional.
func (p *Plugin) probeTool(cfg config.Plugin) error {
	schemaVal, err := p.interp.Eval(p.basePkg + ".Schema")
	if err != nil {
		return nil // no tool export — that's fine
	}

	schemaFn, ok := schemaVal.Interface().(func() sdk.ToolSchema)
	if !ok {
		return fmt.Errorf("plugin %q: Schema has wrong signature, expected func() sdk.ToolSchema", p.name)
	}

	execVal, err := p.interp.Eval(p.basePkg + ".Execute")
	if err != nil {
		return fmt.Errorf("plugin %q: exports Schema but no Execute function", p.name)
	}

	execFn, ok := execVal.Interface().(func(context.Context, sdk.Context, map[string]any) (string, error))
	if !ok {
		return fmt.Errorf(
			"plugin %q: Execute has wrong signature, expected func(context.Context, sdk.Context, map[string]any) (string, error)",
			p.name,
		)
	}

	// Optional: Paths for sandbox + filetime.
	var pathsFn func(map[string]any) (sdk.ToolPaths, error)

	if v, err := p.interp.Eval(p.basePkg + ".Paths"); err == nil {
		if fn, ok := v.Interface().(func(map[string]any) (sdk.ToolPaths, error)); ok {
			pathsFn = fn
		} else {
			fmt.Fprintf(
				os.Stderr,
				"warning: plugin %s exports Paths with wrong signature (got %T), ignoring\n",
				p.name,
				v.Interface(),
			)
		}
	}

	// Optional: Init for runtime config injection.
	if v, err := p.interp.Eval(p.basePkg + ".Init"); err == nil {
		if fn, ok := v.Interface().(func(sdk.ToolConfig)); ok {
			home, _ := folder.Home()
			homeDir := home.Path()
			fn(sdk.ToolConfig{
				HomeDir:   homeDir,
				ConfigDir: p.configDir,
			})
		} else {
			fmt.Fprintf(
				os.Stderr,
				"warning: plugin %s exports Init with wrong signature (got %T), ignoring\n",
				p.name,
				v.Interface(),
			)
		}
	}

	// Optional: Available override (default true).
	available := true

	if v, err := p.interp.Eval(p.basePkg + ".Available"); err == nil {
		if fn, ok := v.Interface().(func() bool); ok {
			available = fn()
		} else {
			fmt.Fprintf(
				os.Stderr,
				"warning: plugin %s exports Available with wrong signature (got %T), ignoring\n",
				p.name,
				v.Interface(),
			)
		}
	}

	// Optional: Sandboxable override (default true).
	sandboxable := true

	if v, err := p.interp.Eval(p.basePkg + ".Sandboxable"); err == nil {
		if fn, ok := v.Interface().(func() bool); ok {
			sandboxable = fn()
		} else {
			fmt.Fprintf(
				os.Stderr,
				"warning: plugin %s exports Sandboxable with wrong signature (got %T), ignoring\n",
				p.name,
				v.Interface(),
			)
		}
	}

	// Optional: Parallel override (default true).
	parallel := true

	if v, err := p.interp.Eval(p.basePkg + ".Parallel"); err == nil {
		if fn, ok := v.Interface().(func() bool); ok {
			parallel = fn()
		} else {
			fmt.Fprintf(
				os.Stderr,
				"warning: plugin %s exports Parallel with wrong signature (got %T), ignoring\n",
				p.name,
				v.Interface(),
			)
		}
	}

	sdkSchema := schemaFn()

	p.tool = &PluginTool{
		Base: tool.Base{
			Text: tool.Text{
				Description: sdkSchema.Description,
				Usage:       sdkSchema.Usage,
				Examples:    sdkSchema.Examples,
			},
		},
		name:         sdkSchema.Name,
		override:     cfg.Override,
		optIn:        cfg.OptIn,
		available:    available,
		sandboxable:  sandboxable,
		parallel:     parallel,
		params:       convertParams(sdkSchema),
		pluginConfig: p.mergedConfig,
		execute:      execFn,
		paths:        pathsFn,
		injectEnv:    p.InjectEnv,
	}

	return nil
}

// convertParams maps SDK tool parameter types to the compiled tool.Parameters.
func convertParams(s sdk.ToolSchema) tool.Parameters {
	props := make(map[string]tool.Property, len(s.Parameters.Properties))
	for name, p := range s.Parameters.Properties {
		props[name] = tool.Property{
			Type:        p.Type,
			Description: p.Description,
			Enum:        p.Enum,
		}
	}

	params := tool.Parameters{
		Type:       s.Parameters.Type,
		Properties: props,
	}

	if len(s.Parameters.Required) > 0 {
		params.Required = s.Parameters.Required
	}

	return params
}
