package injector

import (
	"context"
	"fmt"
	"strings"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/truncate"
	"github.com/idelchi/aura/sdk"
)

// Registry manages registered injectors grouped by timing.
type Registry struct {
	injectors map[Timing][]Injector
}

// New creates a new empty registry.
func New() *Registry {
	return &Registry{
		injectors: make(map[Timing][]Injector),
	}
}

// Register adds an injector to the registry.
func (r *Registry) Register(i Injector) {
	timing := i.Timing()

	r.injectors[timing] = append(r.injectors[timing], i)
}

// Run executes all enabled injectors for a given timing.
// Returns all injections that triggered (may be empty).
func (r *Registry) Run(ctx context.Context, timing Timing, state *State) []Injection {
	var results []Injection

	var checked, fired int

	for _, inj := range r.injectors[timing] {
		if !inj.Enabled() {
			continue
		}

		checked++

		if injection := inj.Check(ctx, state); injection != nil {
			injection.Name = inj.Name()
			fired++

			debug.Log("[injector] %s fired at %s: %s", inj.Name(), timing, truncate.Truncate(injection.Content, 80))

			results = append(results, *injection)
		}
	}

	if checked > 0 {
		debug.Log("[injector] %s: checked=%d fired=%d", timing, checked, fired)
	}

	return results
}

// castTo is a generic type assertion helper for injector interface checks.
func castTo[C any](inj Injector) (C, bool) {
	c, ok := any(inj).(C)

	return c, ok
}

// runCheckers iterates injectors for a timing, type-asserts each to the checker
// interface via cast, calls check, and collects non-nil results.
func runCheckers[C any, R HasBase](
	r *Registry, ctx context.Context, timing Timing,
	cast func(Injector) (C, bool),
	check func(C, context.Context, *State) *R,
	state *State,
) []R {
	var (
		results        []R
		checked, fired int
	)

	for _, inj := range r.injectors[timing] {
		if !inj.Enabled() {
			continue
		}

		checker, ok := cast(inj)
		if !ok {
			continue
		}

		checked++

		result := check(checker, ctx, state)
		if result == nil {
			continue
		}

		fired++

		base := (*result).Base()
		debug.Log("[injector] %s fired at %s: %s", inj.Name(), timing, truncate.Truncate(base.Content, 80))

		results = append(results, *result)
	}

	if checked > 0 {
		debug.Log("[injector] %s: checked=%d fired=%d", timing, checked, fired)
	}

	return results
}

// FiredState returns the names of injectors that have fired (once-per-session).
func (r *Registry) FiredState() map[string]bool {
	state := make(map[string]bool)

	for _, injectors := range r.injectors {
		for _, inj := range injectors {
			if s, ok := inj.(Stateful); ok && s.HasFired() {
				state[inj.Name()] = true
			}
		}
	}

	return state
}

// RestoreFiredState marks injectors as fired based on a previous snapshot.
func (r *Registry) RestoreFiredState(state map[string]bool) {
	for _, injectors := range r.injectors {
		for _, inj := range injectors {
			if s, ok := inj.(Stateful); ok && state[inj.Name()] {
				s.MarkFired(true)
			}
		}
	}
}

// Describer is optionally implemented by injectors that provide extra display info.
type Describer interface {
	Describe() string
}

// Display returns a formatted listing of all registered injectors grouped by timing.
func (r *Registry) Display() string {
	var b strings.Builder

	timings := AllTimings

	first := true

	for _, timing := range timings {
		injectors := r.injectors[timing]
		if len(injectors) == 0 {
			continue
		}

		if !first {
			b.WriteString("\n")
		}

		first = false

		b.WriteString(timing.String() + ":\n")

		for _, inj := range injectors {
			status := "enabled"

			if !inj.Enabled() {
				status = "disabled"
			}

			line := fmt.Sprintf("  %-22s %s", inj.Name(), status)

			if d, ok := inj.(Describer); ok {
				line += "  " + d.Describe()
			}

			b.WriteString(line + "\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// timingNames maps Timing values to human-readable names.
var timingNames = map[Timing]string{
	BeforeChat:          "BeforeChat",
	AfterResponse:       "AfterResponse",
	BeforeToolExecution: "BeforeToolExecution",
	AfterToolExecution:  "AfterToolExecution",
	OnError:             "OnError",
	AfterCompaction:     "AfterCompaction",
	OnAgentSwitch:       "OnAgentSwitch",
	BeforeCompaction:    "BeforeCompaction",
	TransformMessages:   "TransformMessages",
}

// BeforeToolCheckResult holds the aggregated result of all BeforeToolExecution hooks.
type BeforeToolCheckResult struct {
	Arguments map[string]any // final args (chained through all hooks)
	Block     bool           // true if ANY hook blocked
	Messages  []Injection    // messages to inject
}

// RunBeforeTool runs all BeforeToolExecution injectors with argument chaining.
// Each hook sees the modified args from the previous hook. Block is OR'd across all hooks.
func (r *Registry) RunBeforeTool(
	ctx context.Context,
	state *State,
	toolName string,
	args map[string]any,
) BeforeToolCheckResult {
	result := BeforeToolCheckResult{Arguments: args}

	var checked, fired int

	for _, inj := range r.injectors[BeforeToolExecution] {
		if !inj.Enabled() {
			continue
		}

		checker, ok := inj.(BeforeToolChecker)
		if !ok {
			continue
		}

		checked++

		item, err := checker.CheckBeforeTool(ctx, state, toolName, result.Arguments)
		if err != nil {
			debug.Log("[injector] BeforeTool checker %s error: %v", inj.Name(), err)

			continue
		}

		if item == nil {
			continue
		}

		fired++

		if item.Arguments != nil {
			result.Arguments = item.Arguments
		}

		if item.Block {
			result.Block = true
		}

		if item.Injection != nil {
			item.Injection.Name = inj.Name()
			result.Messages = append(result.Messages, *item.Injection)
		}
	}

	if checked > 0 {
		debug.Log(
			"[injector] BeforeToolExecution(%s): checked=%d fired=%d block=%v",
			toolName,
			checked,
			fired,
			result.Block,
		)
	}

	return result
}

// RunBeforeChat runs all BeforeChat injectors and returns typed results.
func (r *Registry) RunBeforeChat(ctx context.Context, state *State) []BeforeChatInjection {
	return runCheckers(r, ctx, BeforeChat, castTo[BeforeChatChecker], BeforeChatChecker.CheckBeforeChat, state)
}

// RunAfterResponse runs all AfterResponse injectors and returns typed results.
func (r *Registry) RunAfterResponse(ctx context.Context, state *State) []AfterResponseInjection {
	return runCheckers(
		r,
		ctx,
		AfterResponse,
		castTo[AfterResponseChecker],
		AfterResponseChecker.CheckAfterResponse,
		state,
	)
}

// RunAfterTool runs all AfterToolExecution injectors and returns typed results.
func (r *Registry) RunAfterTool(ctx context.Context, state *State) []AfterToolInjection {
	return runCheckers(r, ctx, AfterToolExecution, castTo[AfterToolChecker], AfterToolChecker.CheckAfterTool, state)
}

// RunOnError runs all OnError injectors and returns typed results.
func (r *Registry) RunOnError(ctx context.Context, state *State) []OnErrorInjection {
	return runCheckers(r, ctx, OnError, castTo[OnErrorChecker], OnErrorChecker.CheckOnError, state)
}

// RunBeforeCompaction runs all BeforeCompaction injectors and returns typed results.
func (r *Registry) RunBeforeCompaction(ctx context.Context, state *State) []BeforeCompactionInjection {
	return runCheckers(
		r,
		ctx,
		BeforeCompaction,
		castTo[BeforeCompactionChecker],
		BeforeCompactionChecker.CheckBeforeCompaction,
		state,
	)
}

// String returns the human-readable name of a Timing value.
func (t Timing) String() string {
	if name, ok := timingNames[t]; ok {
		return name
	}

	return fmt.Sprintf("Timing(%d)", t)
}

// HasTransformers returns true if any enabled TransformMessages injectors are registered.
func (r *Registry) HasTransformers() bool {
	for _, inj := range r.injectors[TransformMessages] {
		if inj.Enabled() {
			if _, ok := inj.(MessageTransformer); ok {
				return true
			}
		}
	}

	return false
}

// RunTransformMessages runs all TransformMessages injectors as a pipeline.
// Each plugin's output feeds the next plugin's input. Errors are non-fatal — on error,
// the previous result is kept and the next plugin receives it.
func (r *Registry) RunTransformMessages(ctx context.Context, state *State, messages []sdk.Message) []sdk.Message {
	result := messages

	var checked, fired int

	for _, inj := range r.injectors[TransformMessages] {
		if !inj.Enabled() {
			continue
		}

		transformer, ok := inj.(MessageTransformer)
		if !ok {
			continue
		}

		checked++

		transformed, err := transformer.TransformMessages(ctx, state, result)
		if err != nil {
			debug.Log("[injector] TransformMessages %s error: %v", inj.Name(), err)

			continue
		}

		if len(transformed) > 0 {
			fired++

			result = transformed
		}
	}

	if checked > 0 {
		debug.Log("[injector] TransformMessages: checked=%d fired=%d", checked, fired)
	}

	return result
}
