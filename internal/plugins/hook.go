package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/idelchi/aura/internal/condition"
	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/internal/task"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/providers/capabilities"
	"github.com/idelchi/aura/sdk"
	"github.com/idelchi/godyl/pkg/env"
)

// pluginPanic wraps a value recovered from a panicking plugin hook.
type pluginPanic struct {
	value any
}

func (p *pluginPanic) Error() string {
	return fmt.Sprintf("plugin panic: %v", p.value)
}

// safeCall invokes fn with panic recovery. Panics are converted to *pluginPanic errors.
func safeCall[T any](fn func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &pluginPanic{value: r}
		}
	}()

	return fn()
}

// Hook implements injector.Injector by calling a plugin function.
type Hook struct {
	pluginName string
	timing     injector.Timing
	cond       string
	enabled    bool
	once       bool
	fired      atomic.Bool
	panicCount int

	// Exactly one of these is non-nil, matching the timing.
	beforeChat       func(context.Context, sdk.BeforeChatContext) (sdk.Result, error)
	afterResp        func(context.Context, sdk.AfterResponseContext) (sdk.Result, error)
	beforeTool       func(context.Context, sdk.BeforeToolContext) (sdk.BeforeToolResult, error)
	afterTool        func(context.Context, sdk.AfterToolContext) (sdk.Result, error)
	onError          func(context.Context, sdk.OnErrorContext) (sdk.Result, error)
	afterCompaction  func(context.Context, sdk.AfterCompactionContext) (sdk.Result, error)
	onAgentSwitch    func(context.Context, sdk.OnAgentSwitchContext) (sdk.Result, error)
	beforeCompaction func(context.Context, sdk.BeforeCompactionContext) (sdk.Result, error)
	onTransform      func(context.Context, sdk.TransformContext) ([]sdk.Message, error)

	injectEnv    func(env.Env)
	pluginConfig map[string]any
}

// newHook type-asserts a reflect.Value into the correct function type for the timing.
func newHook(
	pluginName string,
	timing injector.Timing,
	cond string,
	enabled, once bool,
	v reflect.Value,
) (*Hook, error) {
	h := &Hook{
		pluginName: pluginName,
		timing:     timing,
		cond:       cond,
		enabled:    enabled,
		once:       once,
	}

	if err := AssignHook(h, timing, v.Interface()); err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Hook) Name() string            { return h.pluginName + "/" + h.timing.String() }
func (h *Hook) Timing() injector.Timing { return h.timing }
func (h *Hook) Enabled() bool           { return h.enabled }
func (h *Hook) HasFired() bool          { return h.fired.Load() }
func (h *Hook) MarkFired(v bool)        { h.fired.Store(v) }
func (h *Hook) PanicCount() int         { return h.panicCount }

// trackPanic inspects err for a recovered panic and, if found, increments the
// panic counter and logs the event.
func (h *Hook) trackPanic(err error) {
	var pe *pluginPanic
	if errors.As(err, &pe) {
		h.panicCount++
		debug.Log("[plugin] %s panicked: %v", h.Name(), pe.value)
	}
}

// Describe returns extra display info for the /plugins listing.
func (h *Hook) Describe() string {
	var parts []string

	if h.cond != "" {
		parts = append(parts, fmt.Sprintf("condition=%q", h.cond))
	}

	if h.once {
		s := "once=true"

		if h.fired.Load() {
			s += " (fired)"
		}

		parts = append(parts, s)
	}

	if h.panicCount > 0 {
		parts = append(parts, fmt.Sprintf("panics=%d", h.panicCount))
	}

	return strings.Join(parts, " ")
}

// prepareAndDispatch runs the common preamble (once-check, condition, env, SDK context)
// and dispatches the hook. Returns (result, error, proceed). If proceed is false, the
// caller should return nil — the hook was skipped (already fired or condition failed).
func (h *Hook) prepareAndDispatch(ctx context.Context, state *injector.State) (sdk.Result, error, bool) {
	if h.once && h.fired.Load() {
		return sdk.Result{}, nil, false
	}

	if h.cond != "" && !condition.Check(h.cond, state.ConditionState()) {
		return sdk.Result{}, nil, false
	}

	if h.injectEnv != nil {
		h.injectEnv(task.EnvFromContext(ctx))
	}

	sdkCtx := BuildSDKContext(state)

	sdkCtx.PluginConfig = h.pluginConfig

	result, err := DispatchHook(h, ctx, state, sdkCtx)

	return result, err, true
}

// Check evaluates the plugin hook and returns an Injection if the hook produces a message.
// Only handles generic timings (AfterCompaction, OnAgentSwitch). All other timings use
// typed CheckXxx methods or the existing CheckBeforeTool/TransformMessages.
func (h *Hook) Check(ctx context.Context, state *injector.State) *injector.Injection {
	if h.timing != injector.AfterCompaction && h.timing != injector.OnAgentSwitch {
		return nil
	}

	result, err, ok := h.prepareAndDispatch(ctx, state)
	if !ok {
		return nil
	}

	if err != nil {
		h.trackPanic(err)
		debug.Log("[plugin] %s hook error: %v", h.Name(), err)

		return &injector.Injection{
			Content:     fmt.Sprintf("plugin hook %q failed: %v", h.Name(), err),
			DisplayOnly: true,
		}
	}

	inj := buildBaseInjection(h, result)
	if inj != nil {
		h.fired.Store(true)
	}

	return inj
}

// buildBaseInjection converts an sdk.Result into a base injector.Injection.
// Returns nil if the result has no message or notice. Timing-specific fields
// are attached by the typed CheckXxx methods.
func buildBaseInjection(h *Hook, result sdk.Result) *injector.Injection {
	if result.Notice != "" {
		return &injector.Injection{
			Name:        h.Name(),
			Content:     result.Notice,
			DisplayOnly: true,
		}
	}

	if result.Message == "" {
		return nil
	}

	role := roles.Assistant

	if result.Role == sdk.RoleUser {
		role = roles.User
	}

	inj := &injector.Injection{
		Name:    h.Name(),
		Role:    role,
		Content: result.Message,
		Prefix:  result.Prefix,
		Eject:   result.Eject,
	}

	if len(result.DisableTools) > 0 {
		inj.Tools = &config.Tools{Disabled: result.DisableTools}
	}

	return inj
}

// CheckBeforeChat implements injector.BeforeChatChecker.
func (h *Hook) CheckBeforeChat(ctx context.Context, state *injector.State) *injector.BeforeChatInjection {
	result, err, ok := h.prepareAndDispatch(ctx, state)
	if !ok {
		return nil
	}

	if err != nil {
		h.trackPanic(err)
		debug.Log("[plugin] %s hook error: %v", h.Name(), err)

		return &injector.BeforeChatInjection{
			Injection: injector.Injection{
				Name:        h.Name(),
				Content:     fmt.Sprintf("plugin hook %q failed: %v", h.Name(), err),
				DisplayOnly: true,
			},
		}
	}

	base := buildBaseInjection(h, result)
	if base == nil && result.Request == nil {
		return nil
	}

	if base == nil {
		base = &injector.Injection{Name: h.Name()}
	}

	h.fired.Store(true)

	return &injector.BeforeChatInjection{
		Injection: *base,
		Request:   result.Request,
	}
}

// CheckAfterResponse implements injector.AfterResponseChecker.
func (h *Hook) CheckAfterResponse(ctx context.Context, state *injector.State) *injector.AfterResponseInjection {
	result, err, ok := h.prepareAndDispatch(ctx, state)
	if !ok {
		return nil
	}

	if err != nil {
		h.trackPanic(err)
		debug.Log("[plugin] %s hook error: %v", h.Name(), err)

		return &injector.AfterResponseInjection{
			Injection: injector.Injection{
				Name:        h.Name(),
				Content:     fmt.Sprintf("plugin hook %q failed: %v", h.Name(), err),
				DisplayOnly: true,
			},
		}
	}

	base := buildBaseInjection(h, result)
	if base == nil && result.Response == nil {
		return nil
	}

	if base == nil {
		base = &injector.Injection{Name: h.Name()}
	}

	h.fired.Store(true)

	return &injector.AfterResponseInjection{
		Injection: *base,
		Response:  result.Response,
	}
}

// CheckAfterTool implements injector.AfterToolChecker.
func (h *Hook) CheckAfterTool(ctx context.Context, state *injector.State) *injector.AfterToolInjection {
	result, err, ok := h.prepareAndDispatch(ctx, state)
	if !ok {
		return nil
	}

	if err != nil {
		h.trackPanic(err)
		debug.Log("[plugin] %s hook error: %v", h.Name(), err)

		return &injector.AfterToolInjection{
			Injection: injector.Injection{
				Name:        h.Name(),
				Content:     fmt.Sprintf("plugin hook %q failed: %v", h.Name(), err),
				DisplayOnly: true,
			},
		}
	}

	base := buildBaseInjection(h, result)
	if base == nil && result.Output == nil {
		return nil
	}

	if base == nil {
		base = &injector.Injection{Name: h.Name()}
	}

	h.fired.Store(true)

	return &injector.AfterToolInjection{
		Injection: *base,
		Output:    result.Output,
	}
}

// CheckOnError implements injector.OnErrorChecker.
func (h *Hook) CheckOnError(ctx context.Context, state *injector.State) *injector.OnErrorInjection {
	result, err, ok := h.prepareAndDispatch(ctx, state)
	if !ok {
		return nil
	}

	if err != nil {
		h.trackPanic(err)
		debug.Log("[plugin] %s hook error: %v", h.Name(), err)

		return &injector.OnErrorInjection{
			Injection: injector.Injection{
				Name:        h.Name(),
				Content:     fmt.Sprintf("plugin hook %q failed: %v", h.Name(), err),
				DisplayOnly: true,
			},
		}
	}

	base := buildBaseInjection(h, result)
	if base == nil && result.Error == nil {
		return nil
	}

	if base == nil {
		base = &injector.Injection{Name: h.Name()}
	}

	h.fired.Store(true)

	return &injector.OnErrorInjection{
		Injection: *base,
		Error:     result.Error,
	}
}

// CheckBeforeCompaction implements injector.BeforeCompactionChecker.
func (h *Hook) CheckBeforeCompaction(ctx context.Context, state *injector.State) *injector.BeforeCompactionInjection {
	result, err, ok := h.prepareAndDispatch(ctx, state)
	if !ok {
		return nil
	}

	if err != nil {
		h.trackPanic(err)
		debug.Log("[plugin] %s hook error: %v", h.Name(), err)

		return &injector.BeforeCompactionInjection{
			Injection: injector.Injection{
				Name:        h.Name(),
				Content:     fmt.Sprintf("plugin hook %q failed: %v", h.Name(), err),
				DisplayOnly: true,
			},
		}
	}

	base := buildBaseInjection(h, result)
	if base == nil && result.Compaction == nil {
		return nil
	}

	if base == nil {
		base = &injector.Injection{Name: h.Name()}
	}

	h.fired.Store(true)

	return &injector.BeforeCompactionInjection{
		Injection:  *base,
		Compaction: result.Compaction,
	}
}

// CheckBeforeTool implements injector.BeforeToolChecker.
// It evaluates the hook with tool-specific context and returns argument modifications
// and/or block decisions instead of a standard Injection.
func (h *Hook) CheckBeforeTool(
	ctx context.Context,
	state *injector.State,
	toolName string,
	args map[string]any,
) (*injector.BeforeToolCheckItem, error) {
	if h.once && h.fired.Load() {
		return nil, nil
	}

	if h.cond != "" && !condition.Check(h.cond, state.ConditionState()) {
		return nil, nil
	}

	// For once-hooks in a concurrent context (Batch sub-tools), CAS after condition
	// check ensures exactly one goroutine fires the hook.
	if h.once && !h.fired.CompareAndSwap(false, true) {
		return nil, nil
	}

	if h.injectEnv != nil {
		h.injectEnv(task.EnvFromContext(ctx))
	}

	sdkCtx := BuildSDKContext(state)

	sdkCtx.PluginConfig = h.pluginConfig

	result, err := safeCall(func() (sdk.BeforeToolResult, error) {
		return h.beforeTool(ctx, sdk.BeforeToolContext{
			Context:   sdkCtx,
			ToolName:  toolName,
			Arguments: args,
		})
	})
	if err != nil {
		h.trackPanic(err)

		return nil, err
	}

	item := &injector.BeforeToolCheckItem{
		Arguments: result.Arguments,
		Block:     result.Block,
		Injection: buildBaseInjection(h, result.Result),
	}

	// No-op: hook ran but produced no modifications (consistent with Check()).
	if item.Arguments == nil && !item.Block && item.Injection == nil {
		return nil, nil
	}

	h.fired.Store(true)

	return item, nil
}

// TransformMessages implements injector.MessageTransformer.
// It evaluates the hook with the message array and returns the transformed result.
func (h *Hook) TransformMessages(
	ctx context.Context,
	state *injector.State,
	messages []sdk.Message,
) ([]sdk.Message, error) {
	if h.once && h.fired.Load() {
		return messages, nil
	}

	if h.cond != "" && !condition.Check(h.cond, state.ConditionState()) {
		return messages, nil
	}

	if h.once && !h.fired.CompareAndSwap(false, true) {
		return messages, nil
	}

	if h.injectEnv != nil {
		h.injectEnv(task.EnvFromContext(ctx))
	}

	sdkCtx := BuildSDKContext(state)

	sdkCtx.PluginConfig = h.pluginConfig

	result, err := safeCall(func() ([]sdk.Message, error) {
		return h.onTransform(ctx, sdk.TransformContext{
			Context:  sdkCtx,
			Messages: messages,
		})
	})
	if err != nil {
		h.trackPanic(err)

		return nil, err
	}

	h.fired.Store(true)

	return result, nil
}

// BuildSDKContext converts injector.State to the SDK Context type.
func BuildSDKContext(state *injector.State) sdk.Context {
	return sdk.Context{
		Iteration: state.Iteration,
		Tokens: sdk.TokenState{
			Estimate: state.Tokens.Estimate,
			LastAPI:  state.Tokens.LastAPI,
			Percent:  state.Tokens.Percent,
			Max:      state.Tokens.Max,
		},
		Agent:        state.Agent,
		Mode:         state.Mode,
		MessageCount: state.MessageCount,
		Workdir:      state.Workdir,

		ModelInfo: sdk.ModelInfo{
			Name:           state.Model.Name,
			Family:         state.Model.Family,
			ParameterCount: int64(state.Model.ParameterCount),
			ContextLength:  int(state.Model.ContextLength),
			Capabilities:   capStrings(state.Model.Capabilities),
		},

		Stats: sdk.Stats{
			StartTime:    state.Stats.StartTime,
			Duration:     state.Stats.Duration(),
			Interactions: state.Stats.Interactions,
			Turns:        state.Stats.Turns,
			Iterations:   state.Stats.Iterations,
			Tools: struct {
				Calls  int
				Errors int
			}{
				Calls:  state.Stats.Tools.Calls,
				Errors: state.Stats.Tools.Errors,
			},
			ParseRetries: state.Stats.ParseRetries,
			Compactions:  state.Stats.Compactions,
			Tokens: struct {
				In  int
				Out int
			}{
				In:  state.Stats.Tokens.In,
				Out: state.Stats.Tokens.Out,
			},
			TopTools: convertTopTools(state.Stats.TopTools(0)),
		},

		Auto:         state.Auto,
		DoneActive:   state.DoneActive,
		HasToolCalls: state.HasToolCalls,
		MaxSteps:     state.MaxSteps,

		Response: struct {
			Empty        bool
			ContentEmpty bool
		}{
			Empty:        state.Response.Empty,
			ContentEmpty: state.Response.ContentEmpty,
		},

		Todo: struct {
			Pending    int
			InProgress int
			Total      int
		}{
			Pending:    state.Todo.Pending,
			InProgress: state.Todo.InProgress,
			Total:      state.Todo.Total,
		},

		PatchCounts: state.PatchCounts,
		ToolHistory: convertToolHistory(state.ToolHistory),

		Session: struct {
			ID    string
			Title string
		}{
			ID:    state.Session.ID,
			Title: state.Session.Title,
		},
		Provider:  state.Provider,
		ThinkMode: state.ThinkMode,
		Features: sdk.FeatureState{
			Sandbox: sdk.SandboxFeatureState{
				Enabled:   state.Sandbox.Enabled,
				Requested: state.Sandbox.Requested,
			},
			ReadBeforeWrite:   state.ReadBeforeWrite,
			ShowThinking:      state.ShowThinking,
			CompactionEnabled: state.Compaction.Enabled,
		},
		AvailableTools: state.AvailableTools,
		LoadedTools:    state.LoadedTools,
		Turns:          state.Turns,
		SystemPrompt:   state.SystemPrompt,
		MCPServers:     state.MCPServers,
		Vars:           state.Vars,
	}
}

// convertToolHistory converts injector.ToolCall slice to sdk.ToolCall slice.
func convertToolHistory(history []injector.ToolCall) []sdk.ToolCall {
	if len(history) == 0 {
		return nil
	}

	result := make([]sdk.ToolCall, len(history))
	for i, tc := range history {
		argsJSON, err := json.Marshal(tc.Args)
		if err != nil {
			debug.Log("[hook] marshal tool args %s: %v", tc.Name, err)
		}

		result[i] = sdk.ToolCall{
			Name:     tc.Name,
			Args:     tc.Args,
			ArgsJSON: string(argsJSON),
			Result:   tc.Result,
			Error:    tc.Error,
			Duration: tc.Duration,
		}
	}

	return result
}

// convertTopTools converts stats.ToolCount slice to sdk.ToolCount slice.
func convertTopTools(tools []stats.ToolCount) []sdk.ToolCount {
	if len(tools) == 0 {
		return nil
	}

	result := make([]sdk.ToolCount, len(tools))
	for i, tc := range tools {
		result[i] = sdk.ToolCount{Name: tc.Name, Count: tc.Count}
	}

	return result
}

// capStrings converts capabilities to a plain string slice for the SDK.
func capStrings(caps capabilities.Capabilities) []string {
	if len(caps) == 0 {
		return nil
	}

	result := make([]string, len(caps))
	for i, c := range caps {
		result[i] = string(c)
	}

	return result
}
