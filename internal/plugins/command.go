package plugins

import (
	"context"
	"strings"

	"github.com/idelchi/aura/internal/slash"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/sdk"
	"github.com/idelchi/godyl/pkg/env"
)

// PluginCommand wraps a Yaegi-interpreted plugin's command exports.
type PluginCommand struct {
	schema       sdk.CommandSchema
	execute      func(context.Context, string, sdk.Context) (sdk.CommandResult, error)
	injectEnv    func(env.Env)
	pluginConfig map[string]any
}

// Name returns the command name with "/" prefix.
func (pc *PluginCommand) Name() string { return "/" + pc.schema.Name }

// ToSlashCommand converts the plugin command to a slash.Command for registration.
func (pc *PluginCommand) ToSlashCommand() slash.Command {
	return slash.Command{
		Name:        "/" + pc.schema.Name,
		Description: pc.schema.Description,
		Hints:       pc.schema.Hints,
		Forward:     pc.schema.Forward,
		Silent:      pc.schema.Silent,
		Execute: func(ctx context.Context, sctx slash.Context, args ...string) (string, error) {
			sdkCtx := buildSDKContextFromSlash(sctx)

			sdkCtx.PluginConfig = pc.pluginConfig

			if pc.injectEnv != nil {
				pc.injectEnv(task.EnvFromContext(ctx))
			}

			result, err := safeCall(func() (sdk.CommandResult, error) {
				return pc.execute(ctx, strings.Join(args, " "), sdkCtx)
			})
			if err != nil {
				return "", err
			}

			return result.Output, nil
		},
	}
}

// buildSDKContextFromSlash constructs an sdk.Context from slash.Context.
// Unlike BuildSDKContext (which reads injector.State), this reads from
// the slash.Context methods available to commands at runtime.
//
// Missing fields vs hook context: MessageCount, ModelInfo, Workdir,
// ToolHistory, PatchCounts — commands run between turns so these are
// unavailable or stale. The subset is sufficient for stateful commands.
func buildSDKContextFromSlash(sctx slash.Context) sdk.Context {
	r := sctx.Resolved()
	status := sctx.Status()
	st := sctx.SessionStats()

	return sdk.Context{
		Iteration: status.Steps.Current,
		Tokens: sdk.TokenState{
			Estimate: status.Tokens.Used,
			Percent:  status.Tokens.Percent,
			Max:      status.Tokens.Max,
		},
		Agent:    r.Agent,
		Mode:     r.Mode,
		Auto:     r.Auto,
		MaxSteps: status.Steps.Max,
		Stats: sdk.Stats{
			StartTime:    st.StartTime,
			Duration:     st.Duration(),
			Interactions: st.Interactions,
			Turns:        st.Turns,
			Iterations:   st.Iterations,
			Tools: struct {
				Calls  int
				Errors int
			}{
				Calls:  st.Tools.Calls,
				Errors: st.Tools.Errors,
			},
			ParseRetries: st.ParseRetries,
			Compactions:  st.Compactions,
			Tokens: struct {
				In  int
				Out int
			}{
				In:  st.Tokens.In,
				Out: st.Tokens.Out,
			},
		},
	}
}
