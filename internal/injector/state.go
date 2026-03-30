package injector

import (
	"time"

	"github.com/idelchi/aura/internal/condition"
	"github.com/idelchi/aura/internal/stats"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/sdk"
)

// SandboxState holds sandbox toggle state for plugin context.
type SandboxState struct {
	Enabled   bool // Landlock actually enforcing
	Requested bool // User wants sandbox (may not be enforced)
}

// ToolCall captures a single tool invocation.
type ToolCall struct {
	Name     string
	Args     map[string]any
	Result   string
	Error    string
	Duration time.Duration
}

// TokenSnapshot holds token measurements frozen at state build time.
type TokenSnapshot struct {
	Estimate int     // Client-side estimate: per-message sum + tool schema tokens (forward-looking)
	LastAPI  int     // API-reported input tokens from last provider chat() call (backward-looking)
	Percent  float64 // 0-100 range, based on Estimate
	Max      int     // Effective context window size in tokens
}

// ResponseState holds the current response metadata.
type ResponseState struct {
	Content      string     // response text content
	Thinking     string     // thinking block from this response
	Calls        []ToolCall // pending tool calls from this response (pre-execution)
	Empty        bool
	ContentEmpty bool
}

// TodoState holds todo list counts.
type TodoState struct {
	Pending    int
	InProgress int
	Total      int
}

// CompactionState holds compaction event data.
type CompactionState struct {
	Enabled       bool
	Forced        bool // true if triggered by explicit /compact or force=true
	Success       bool
	PreMessages   int
	PostMessages  int
	SummaryLength int
	KeepLast      int // how many recent messages will be preserved
}

// AgentSwitchState holds agent switch event data.
type AgentSwitchState struct {
	Previous string
	New      string
	Reason   string
}

// ErrorState holds classified error metadata.
type ErrorState struct {
	Type      string // classified error type (e.g. "rate_limit", "network")
	Retryable bool   // true for transient errors
}

// SessionState holds session identity.
type SessionState struct {
	ID    string
	Title string
}

// State provides context about the current conversation state.
type State struct {
	Iteration    int
	ToolHistory  []ToolCall
	Response     ResponseState
	HasToolCalls bool
	Todo         TodoState
	Mode         string
	Auto         bool
	PatchCounts  map[string]int
	Error        error
	ErrorInfo    ErrorState

	Tokens       TokenSnapshot
	MessageCount int

	// Value snapshots — frozen at build time, safe to read without mutation risk.
	Stats stats.Snapshot
	Model model.Model

	// Plugin SDK fields — used by plugin hooks to populate sdk.Context.
	Agent      string
	Workdir    string
	DoneActive bool
	MaxSteps   int

	Session         SessionState
	Provider        string
	ThinkMode       string
	Sandbox         SandboxState
	ReadBeforeWrite bool
	ShowThinking    bool
	Compaction      CompactionState

	AgentSwitch AgentSwitchState

	AvailableTools []string
	LoadedTools    []string
	Turns          []sdk.Turn
	SystemPrompt   string
	MCPServers     []string
	Vars           map[string]string
}

// ConditionState converts the injector state to a condition evaluation state.
func (s *State) ConditionState() condition.State {
	return condition.State{
		Todo: condition.TodoState{
			Empty:   s.Todo.Total == 0,
			Done:    s.Todo.Total > 0 && s.Todo.Pending == 0 && s.Todo.InProgress == 0,
			Pending: s.Todo.Pending > 0 || s.Todo.InProgress > 0,
		},
		Tokens: condition.TokensState{
			Percent: s.Tokens.Percent,
			Total:   s.Stats.Tokens.In + s.Stats.Tokens.Out,
		},
		MessageCount: s.MessageCount,
		Tools: condition.ToolsState{
			Errors: s.Stats.Tools.Errors,
			Calls:  s.Stats.Tools.Calls,
		},
		Turns:       s.Stats.Turns,
		Compactions: s.Stats.Compactions,
		Iteration:   s.Iteration,
		Auto:        s.Auto,
		Model: condition.ModelState{
			ParamCount:    int64(s.Model.ParameterCount),
			ContextLength: int(s.Model.ContextLength),
			Capabilities:  s.Model.Capabilities.Map(),
			Name:          s.Model.Name,
		},
	}
}

// LastTool returns the most recent tool call, or nil if none.
func (s *State) LastTool() *ToolCall {
	if len(s.ToolHistory) == 0 {
		return nil
	}

	return &s.ToolHistory[len(s.ToolHistory)-1]
}

// PrevTool returns the second-to-last tool call, or nil if none.
func (s *State) PrevTool() *ToolCall {
	if len(s.ToolHistory) < 2 {
		return nil
	}

	return &s.ToolHistory[len(s.ToolHistory)-2]
}
