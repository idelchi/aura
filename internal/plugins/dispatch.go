package plugins

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/sdk"
)

// timingEntry defines how a single timing is assigned and dispatched.
type timingEntry struct {
	Name     string
	Timing   injector.Timing
	assign   func(h *Hook, iface any) error
	dispatch func(h *Hook, ctx context.Context, state *injector.State, sdkCtx sdk.Context) (sdk.Result, error)
}

// timingRegistry is the single source of truth for all hook timings.
// Order matters — it defines the probing order used by KnownTimingNames().
var timingRegistry = []timingEntry{
	{
		Name:   "BeforeChat",
		Timing: injector.BeforeChat,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.BeforeChatContext) (sdk.Result, error))
			if !ok {
				return fmt.Errorf(
					"BeforeChat has wrong signature %T, expected func(context.Context, sdk.BeforeChatContext) (sdk.Result, error)",
					iface,
				)
			}

			h.beforeChat = fn

			return nil
		},
		dispatch: func(h *Hook, ctx context.Context, _ *injector.State, sdkCtx sdk.Context) (sdk.Result, error) {
			return safeCall(func() (sdk.Result, error) {
				return h.beforeChat(ctx, sdk.BeforeChatContext{Context: sdkCtx})
			})
		},
	},
	{
		Name:   "AfterResponse",
		Timing: injector.AfterResponse,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.AfterResponseContext) (sdk.Result, error))
			if !ok {
				return fmt.Errorf(
					"AfterResponse has wrong signature %T, expected func(context.Context, sdk.AfterResponseContext) (sdk.Result, error)",
					iface,
				)
			}

			h.afterResp = fn

			return nil
		},
		dispatch: func(h *Hook, ctx context.Context, state *injector.State, sdkCtx sdk.Context) (sdk.Result, error) {
			return safeCall(func() (sdk.Result, error) {
				return h.afterResp(ctx, sdk.AfterResponseContext{
					Context:  sdkCtx,
					Content:  state.Response.Content,
					Thinking: state.Response.Thinking,
					Calls:    convertToolHistory(state.Response.Calls),
				})
			})
		},
	},
	{
		Name:   "BeforeToolExecution",
		Timing: injector.BeforeToolExecution,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.BeforeToolContext) (sdk.BeforeToolResult, error))
			if !ok {
				return fmt.Errorf(
					"BeforeToolExecution has wrong signature %T, expected func(context.Context, sdk.BeforeToolContext) (sdk.BeforeToolResult, error)",
					iface,
				)
			}

			h.beforeTool = fn

			return nil
		},
		dispatch: func(_ *Hook, _ context.Context, _ *injector.State, _ sdk.Context) (sdk.Result, error) {
			return sdk.Result{}, nil // handled via CheckBeforeTool()
		},
	},
	{
		Name:   "AfterToolExecution",
		Timing: injector.AfterToolExecution,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.AfterToolContext) (sdk.Result, error))
			if !ok {
				return fmt.Errorf(
					"AfterToolExecution has wrong signature %T, expected func(context.Context, sdk.AfterToolContext) (sdk.Result, error)",
					iface,
				)
			}

			h.afterTool = fn

			return nil
		},
		dispatch: func(h *Hook, ctx context.Context, state *injector.State, sdkCtx sdk.Context) (sdk.Result, error) {
			actx := sdk.AfterToolContext{Context: sdkCtx}

			if tc := state.LastTool(); tc != nil {
				actx.Tool.Name = tc.Name
				actx.Tool.Result = tc.Result
				actx.Tool.Error = tc.Error
				actx.Tool.Duration = tc.Duration
			}

			return safeCall(func() (sdk.Result, error) {
				return h.afterTool(ctx, actx)
			})
		},
	},
	{
		Name:   "OnError",
		Timing: injector.OnError,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.OnErrorContext) (sdk.Result, error))
			if !ok {
				return fmt.Errorf(
					"OnError has wrong signature %T, expected func(context.Context, sdk.OnErrorContext) (sdk.Result, error)",
					iface,
				)
			}

			h.onError = fn

			return nil
		},
		dispatch: func(h *Hook, ctx context.Context, state *injector.State, sdkCtx sdk.Context) (sdk.Result, error) {
			errStr := ""

			if state.Error != nil {
				errStr = state.Error.Error()
			}

			return safeCall(func() (sdk.Result, error) {
				return h.onError(ctx, sdk.OnErrorContext{
					Context:    sdkCtx,
					Error:      errStr,
					ErrorType:  state.ErrorInfo.Type,
					Retryable:  state.ErrorInfo.Retryable,
					StatusCode: 0,
				})
			})
		},
	},
	{
		Name:   "BeforeCompaction",
		Timing: injector.BeforeCompaction,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.BeforeCompactionContext) (sdk.Result, error))
			if !ok {
				return fmt.Errorf(
					"BeforeCompaction has wrong signature %T, expected func(context.Context, sdk.BeforeCompactionContext) (sdk.Result, error)",
					iface,
				)
			}

			h.beforeCompaction = fn

			return nil
		},
		dispatch: func(h *Hook, ctx context.Context, state *injector.State, sdkCtx sdk.Context) (sdk.Result, error) {
			return safeCall(func() (sdk.Result, error) {
				return h.beforeCompaction(ctx, sdk.BeforeCompactionContext{
					Context:        sdkCtx,
					Forced:         state.Compaction.Forced,
					TokensUsed:     state.Tokens.Estimate,
					ContextPercent: state.Tokens.Percent,
					MessageCount:   state.MessageCount,
					KeepLast:       state.Compaction.KeepLast,
				})
			})
		},
	},
	{
		Name:   "AfterCompaction",
		Timing: injector.AfterCompaction,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.AfterCompactionContext) (sdk.Result, error))
			if !ok {
				return fmt.Errorf(
					"AfterCompaction has wrong signature %T, expected func(context.Context, sdk.AfterCompactionContext) (sdk.Result, error)",
					iface,
				)
			}

			h.afterCompaction = fn

			return nil
		},
		dispatch: func(h *Hook, ctx context.Context, state *injector.State, sdkCtx sdk.Context) (sdk.Result, error) {
			return safeCall(func() (sdk.Result, error) {
				return h.afterCompaction(ctx, sdk.AfterCompactionContext{
					Context:       sdkCtx,
					Success:       state.Compaction.Success,
					PreMessages:   state.Compaction.PreMessages,
					PostMessages:  state.Compaction.PostMessages,
					SummaryLength: state.Compaction.SummaryLength,
				})
			})
		},
	},
	{
		Name:   "OnAgentSwitch",
		Timing: injector.OnAgentSwitch,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.OnAgentSwitchContext) (sdk.Result, error))
			if !ok {
				return fmt.Errorf(
					"OnAgentSwitch has wrong signature %T, expected func(context.Context, sdk.OnAgentSwitchContext) (sdk.Result, error)",
					iface,
				)
			}

			h.onAgentSwitch = fn

			return nil
		},
		dispatch: func(h *Hook, ctx context.Context, state *injector.State, sdkCtx sdk.Context) (sdk.Result, error) {
			return safeCall(func() (sdk.Result, error) {
				return h.onAgentSwitch(ctx, sdk.OnAgentSwitchContext{
					Context:       sdkCtx,
					PreviousAgent: state.AgentSwitch.Previous,
					NewAgent:      state.AgentSwitch.New,
					Reason:        state.AgentSwitch.Reason,
				})
			})
		},
	},
	{
		Name:   "TransformMessages",
		Timing: injector.TransformMessages,
		assign: func(h *Hook, iface any) error {
			fn, ok := iface.(func(context.Context, sdk.TransformContext) ([]sdk.Message, error))
			if !ok {
				return fmt.Errorf(
					"TransformMessages has wrong signature %T, expected func(context.Context, sdk.TransformContext) ([]sdk.Message, error)",
					iface,
				)
			}

			h.onTransform = fn

			return nil
		},
		dispatch: func(_ *Hook, _ context.Context, _ *injector.State, _ sdk.Context) (sdk.Result, error) {
			return sdk.Result{}, nil // handled via TransformMessages() method
		},
	},
}

// AssignHook type-asserts iface and assigns to the appropriate function field on h.
func AssignHook(h *Hook, timing injector.Timing, iface any) error {
	for _, entry := range timingRegistry {
		if entry.Timing == timing {
			return entry.assign(h, iface)
		}
	}

	return fmt.Errorf("unknown timing %v", timing)
}

// DispatchHook calls the appropriate hook function based on timing.
// BeforeToolExecution is NOT dispatched here — it has a separate CheckBeforeTool() path
// that returns BeforeToolCheckItem (not sdk.Result). Check() returns nil for that timing.
func DispatchHook(h *Hook, ctx context.Context, state *injector.State, sdkCtx sdk.Context) (sdk.Result, error) {
	for _, entry := range timingRegistry {
		if entry.Timing == h.timing {
			return entry.dispatch(h, ctx, state, sdkCtx)
		}
	}

	return sdk.Result{}, fmt.Errorf("unknown timing %v", h.timing)
}

// KnownTimingNames returns the ordered list of timing names for plugin probing.
func KnownTimingNames() []struct {
	Name   string
	Timing injector.Timing
} {
	result := make([]struct {
		Name   string
		Timing injector.Timing
	}, len(timingRegistry))

	for i, entry := range timingRegistry {
		result[i] = struct {
			Name   string
			Timing injector.Timing
		}{entry.Name, entry.Timing}
	}

	return result
}
