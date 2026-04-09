package injector

import (
	"context"
	"fmt"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/truncate"
	"github.com/idelchi/aura/sdk"
)

// Timing defines when an injector runs.
type Timing int

const (
	// BeforeChat runs before sending to LLM.
	BeforeChat Timing = iota
	// AfterResponse runs after receiving LLM response.
	AfterResponse
	// BeforeToolExecution runs before each tool execution.
	BeforeToolExecution
	// AfterToolExecution runs after each tool execution.
	AfterToolExecution
	// OnError runs when an error occurs.
	OnError
	// AfterCompaction runs after context compaction completes.
	AfterCompaction
	// OnAgentSwitch runs when the active agent changes.
	OnAgentSwitch
	// BeforeCompaction runs before context compaction begins.
	BeforeCompaction
	// TransformMessages runs inside chat() to transform the message array before the LLM call.
	TransformMessages
)

// AllTimings is the canonical ordered list of every Timing value.
var AllTimings = []Timing{
	BeforeChat,
	AfterResponse,
	BeforeToolExecution,
	AfterToolExecution,
	OnError,
	BeforeCompaction,
	AfterCompaction,
	OnAgentSwitch,
	TransformMessages,
}

// maxDisplayLen is the maximum length for truncated display content.
const maxDisplayLen = 120

// Injection represents the base message to inject. Timing-specific data lives in
// typed structs (BeforeChatInjection, AfterResponseInjection, etc.) that embed this.
type Injection struct {
	Name        string // Injector name (e.g. "loop_detection", "max_steps")
	Role        roles.Role
	Content     string
	Prefix      string        // e.g., "[SYSTEM FEEDBACK]: "
	Eject       bool          // remove after one turn
	Tools       *config.Tools // tool filter for the turn this injection fires (nil = no override)
	DisplayOnly bool          // show in UI but don't add to conversation history
}

// BeforeChatInjection is returned by BeforeChat hooks.
type BeforeChatInjection struct {
	Injection

	Request *sdk.RequestModification
}

// AfterResponseInjection is returned by AfterResponse hooks.
type AfterResponseInjection struct {
	Injection

	Response *sdk.ResponseModification
}

// AfterToolInjection is returned by AfterToolExecution hooks.
type AfterToolInjection struct {
	Injection

	Output *string
}

// OnErrorInjection is returned by OnError hooks.
type OnErrorInjection struct {
	Injection

	Error *sdk.ErrorModification
}

// BeforeCompactionInjection is returned by BeforeCompaction hooks.
type BeforeCompactionInjection struct {
	Injection

	Compaction *sdk.CompactionModification
}

// HasBase is implemented by typed injection structs to extract their base Injection.
type HasBase interface {
	Base() Injection
}

// Base returns the embedded Injection.
func (inj BeforeChatInjection) Base() Injection { return inj.Injection }

// Base returns the embedded Injection.
func (inj AfterResponseInjection) Base() Injection { return inj.Injection }

// Base returns the embedded Injection.
func (inj AfterToolInjection) Base() Injection { return inj.Injection }

// Base returns the embedded Injection.
func (inj OnErrorInjection) Base() Injection { return inj.Injection }

// Base returns the embedded Injection.
func (inj BeforeCompactionInjection) Base() Injection { return inj.Injection }

// Bases extracts base Injection values from a typed injection slice.
// Go has no slice covariance, so this bridges typed slices to []Injection.
func Bases[T HasBase](items []T) []Injection {
	if len(items) == 0 {
		return nil
	}

	result := make([]Injection, len(items))
	for i, item := range items {
		result[i] = item.Base()
	}

	return result
}

// DisplayHeader returns the formatted header for UI display.
func (inj Injection) DisplayHeader() string {
	if inj.DisplayOnly {
		return "[NOTICE]: " + inj.Name
	}

	return fmt.Sprintf("[SYNTHETIC %s]: %s", inj.Role, inj.Name)
}

// DisplayContent returns the injection content for UI display.
// Display-only injections are shown in full; conversation injections are truncated.
func (inj Injection) DisplayContent() string {
	content := inj.Prefix + inj.Content
	if inj.DisplayOnly {
		return content
	}

	return truncate.Truncate(content, maxDisplayLen)
}

// Stateful is optionally implemented by injectors that track once-per-session state.
// Used to preserve fired state across config reloads.
type Stateful interface {
	HasFired() bool
	MarkFired(bool)
}

// BeforeToolChecker is implemented by injectors that support BeforeToolExecution timing.
// Defined here (not in plugins) to avoid an import cycle: registry type-asserts to this interface.
type BeforeToolChecker interface {
	CheckBeforeTool(
		ctx context.Context,
		state *State,
		toolName string,
		args map[string]any,
	) (*BeforeToolCheckItem, error)
}

// MessageTransformer is implemented by injectors that support TransformMessages timing.
// Returns a modified message array for the current LLM call (ephemeral — builder history untouched).
type MessageTransformer interface {
	TransformMessages(ctx context.Context, state *State, messages []sdk.Message) ([]sdk.Message, error)
}

// BeforeChatChecker is implemented by injectors that support BeforeChat timing.
type BeforeChatChecker interface {
	CheckBeforeChat(ctx context.Context, state *State) *BeforeChatInjection
}

// AfterResponseChecker is implemented by injectors that support AfterResponse timing.
type AfterResponseChecker interface {
	CheckAfterResponse(ctx context.Context, state *State) *AfterResponseInjection
}

// AfterToolChecker is implemented by injectors that support AfterToolExecution timing.
type AfterToolChecker interface {
	CheckAfterTool(ctx context.Context, state *State) *AfterToolInjection
}

// OnErrorChecker is implemented by injectors that support OnError timing.
type OnErrorChecker interface {
	CheckOnError(ctx context.Context, state *State) *OnErrorInjection
}

// BeforeCompactionChecker is implemented by injectors that support BeforeCompaction timing.
type BeforeCompactionChecker interface {
	CheckBeforeCompaction(ctx context.Context, state *State) *BeforeCompactionInjection
}

// BeforeToolCheckItem holds one hook's BeforeToolExecution result.
type BeforeToolCheckItem struct {
	Arguments map[string]any // if non-nil, replaces args for subsequent hooks
	Block     bool           // if true, skip tool execution
	Injection *Injection     // message to inject (nil = none)
}

// Injector produces messages to inject into the conversation.
type Injector interface {
	// Name returns a unique identifier for this injector.
	Name() string

	// Timing returns when this injector should run.
	Timing() Timing

	// Check determines if injection is needed given current state.
	// Returns nil if no injection needed.
	Check(ctx context.Context, state *State) *Injection

	// Enabled returns whether this injector is active.
	Enabled() bool
}
