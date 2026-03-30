package sdk

import (
	"time"

	// Blank import ensures sdk/version is in the transitive import graph,
	// so plugins that vendor sdk automatically get sdk/version too.
	_ "github.com/idelchi/aura/sdk/version"
)

// Role identifies the speaker for an injected message.
type Role string

const (
	// RoleUser injects as a user message.
	RoleUser Role = "user"
	// RoleAssistant injects as an assistant message.
	RoleAssistant Role = "assistant"
)

// Result is the return value from a hook function.
// A zero Result means "no injection" — the hook ran but has nothing to say.
type Result struct {
	// Message is the content to inject into the conversation.
	// Empty string means no injection.
	Message string
	// Role is the message role (user or assistant). Defaults to RoleAssistant if empty.
	Role Role
	// Prefix is prepended to Message (e.g. "[SYSTEM FEEDBACK]: ").
	Prefix string
	// Eject removes the injected message after one turn.
	Eject bool
	// DisableTools lists tool name patterns to disable (e.g. ["*"] to disable all).
	DisableTools []string
	// Notice is display-only text shown in the TUI but not sent to the LLM.
	// When set, Message is ignored.
	Notice string
	// Output, if non-nil, replaces tool output sent to the LLM (AfterToolExecution only).
	Output *string
	// Response, if non-nil, controls how the LLM response enters conversation history (AfterResponse only).
	Response *ResponseModification
	// Error, if non-nil, controls how the error is handled (OnError only).
	Error *ErrorModification
	// Request, if non-nil, controls the chat request (BeforeChat only).
	Request *RequestModification
	// Compaction, if non-nil, controls compaction behavior (BeforeCompaction only).
	Compaction *CompactionModification
}

// ResponseModification controls how the LLM response enters conversation history.
// Only processed for AfterResponse timing.
type ResponseModification struct {
	// Skip prevents the response from being added to conversation history.
	// The response was already streamed to the UI — skip just removes it from future context.
	Skip bool
	// Content, if non-nil, replaces the response content before adding to history.
	Content *string
}

// ErrorModification controls how the assistant loop handles an error.
// Only processed for OnError timing.
type ErrorModification struct {
	// Retry requests the loop retry the chat() call without counting as a new iteration.
	Retry bool
	// Skip suppresses the error — the loop returns nil instead of the error.
	Skip bool
}

// RequestModification controls the chat request before it is sent.
// Only processed for BeforeChat timing.
type RequestModification struct {
	// AppendSystem, if non-nil, appends text to the system prompt for this turn only (not persisted).
	AppendSystem *string
	// Skip prevents chat() from being called — the turn ends immediately.
	Skip bool
}

// ToolCall captures a single tool invocation.
type ToolCall struct {
	Name     string
	Args     map[string]any
	ArgsJSON string // JSON-encoded Args for reliable comparison
	Result   string
	Error    string
	Duration time.Duration // wall-clock execution time
}

// TokenState holds token usage from multiple measurement sources.
type TokenState struct {
	Estimate int     // Client-side estimate: per-message sum + tool schema tokens (forward-looking, current conversation size)
	LastAPI  int     // API-reported input tokens from last provider chat() call (backward-looking, what the provider counted)
	Percent  float64 // 0-100 range, based on Estimate
	Max      int     // Effective context window size in tokens
}

// Stats holds cumulative session metrics.
type Stats struct {
	StartTime    time.Time
	Duration     time.Duration
	Interactions int
	Turns        int
	Iterations   int
	Tools        struct {
		Calls  int
		Errors int
	}
	ParseRetries int
	Compactions  int
	Tokens       struct {
		In  int
		Out int
	}
	TopTools []ToolCount
}

// ToolCount pairs a tool name with its invocation count.
type ToolCount struct {
	Name  string
	Count int
}

// ModelInfo describes the resolved model.
type ModelInfo struct {
	Name           string
	Family         string
	ParameterCount int64
	ContextLength  int
	Capabilities   []string
}

// FeatureState holds runtime feature toggle states.
type FeatureState struct {
	Sandbox           SandboxFeatureState // Sandbox toggle state
	ReadBeforeWrite   bool                // Read-before-write enforcement active
	ShowThinking      bool                // Thinking display in TUI (/verbose toggle)
	CompactionEnabled bool                // Whether compaction is configured
}

// SandboxFeatureState holds sandbox toggle state for plugin consumption.
type SandboxFeatureState struct {
	Enabled   bool // Landlock actually enforcing
	Requested bool // User wants sandbox (may not be enforced if Landlock unavailable)
}

// Turn captures a single conversation turn (user or assistant text only).
type Turn struct {
	Role    string // "user" or "assistant"
	Content string
}

// Context holds runtime state available to all hooks.
type Context struct {
	Iteration int
	Tokens    TokenState
	Agent     string
	Mode      string
	ModelInfo ModelInfo
	Stats     Stats
	Workdir   string

	MessageCount int

	// Conversation state
	Auto         bool
	DoneActive   bool
	HasToolCalls bool
	MaxSteps     int

	Response struct {
		Empty        bool
		ContentEmpty bool
	}

	Todo struct {
		Pending    int
		InProgress int
		Total      int
	}

	// Tool/patch tracking
	PatchCounts map[string]int
	ToolHistory []ToolCall

	// Session identity
	Session struct {
		ID    string // UUID of active session (empty if unsaved)
		Title string // User-set or auto-generated title
	}

	// Provider & runtime
	Provider  string // Active provider name (e.g., "anthropic", "ollama")
	ThinkMode string // Thinking mode: "off", "low", "medium", "high"

	// Feature state
	Features FeatureState

	// Tool awareness
	AvailableTools []string // Names of all tools available to the agent (after filtering)
	LoadedTools    []string // Deferred tools explicitly loaded this session

	// Conversation turns (text-only user/assistant messages, excludes system/tool/synthetic/ephemeral)
	Turns []Turn

	// System prompt sent to the model
	SystemPrompt string

	// Connected MCP server names
	MCPServers []string

	// Template variables from --set flags
	Vars map[string]string

	// PluginConfig holds the merged config values for the calling plugin.
	PluginConfig map[string]any
}

// BeforeChatContext is passed to BeforeChat hooks.
type BeforeChatContext struct {
	Context
}

// AfterResponseContext is passed to AfterResponse hooks.
type AfterResponseContext struct {
	Context
	Content  string     // response text content
	Thinking string     // thinking block text from this response
	Calls    []ToolCall // pending tool calls from this response (unexecuted: Result/Error/Duration are zero)
}

// BeforeToolContext is passed to BeforeToolExecution hooks.
type BeforeToolContext struct {
	Context
	ToolName  string
	Arguments map[string]any
}

// BeforeToolResult is returned by BeforeToolExecution hooks.
type BeforeToolResult struct {
	Result
	// Arguments, if non-nil, replaces tool arguments before execution.
	Arguments map[string]any
	// Block, if true, skips tool execution entirely.
	Block bool
}

// AfterToolContext is passed to AfterToolExecution hooks.
type AfterToolContext struct {
	Context
	Tool struct {
		Name     string
		Result   string
		Error    string
		Duration time.Duration
	}
}

// OnErrorContext is passed to OnError hooks.
type OnErrorContext struct {
	Context
	Error      string // raw error message
	ErrorType  string // classified type: "rate_limit", "auth", "network", "server", "content_filter", "credit_exhausted", "model_unavailable", "context_exhausted", or ""
	Retryable  bool   // true for transient errors (rate_limit, server, network)
	StatusCode int    // HTTP status code (0 if unavailable — providers discard status after classification)
}

// AfterCompactionContext is passed to AfterCompaction hooks.
type AfterCompactionContext struct {
	Context
	Success       bool // whether compaction produced a summary
	PreMessages   int  // message count before compaction
	PostMessages  int  // message count after rebuild (0 on failure)
	SummaryLength int  // character length of compaction summary (0 on failure)
}

// OnAgentSwitchContext is passed to OnAgentSwitch hooks.
type OnAgentSwitchContext struct {
	Context
	PreviousAgent string // agent name before switch
	NewAgent      string // agent name after switch
	Reason        string // "user", "failover", "task", "cycle", "resume"
}

// ToolSchema declares a tool's function-calling schema.
type ToolSchema struct {
	Name        string
	Description string
	Usage       string
	Examples    string
	Parameters  ToolParameters
}

// ToolParameters describes the parameter object.
type ToolParameters struct {
	Type       string // always "object"
	Properties map[string]ToolProperty
	Required   []string
}

// ToolProperty describes a single parameter.
type ToolProperty struct {
	Type        string
	Description string
	Enum        []any
}

// ToolConfig provides runtime paths to plugin tools at initialization.
// Passed to the optional Init(ToolConfig) export once at load time.
type ToolConfig struct {
	HomeDir   string // user home directory (~)
	ConfigDir string // project .aura/ directory
}

// ToolPaths declares filesystem paths a tool will access.
type ToolPaths struct {
	Read   []string // sandbox: paths the tool will read
	Write  []string // sandbox: paths the tool will write
	Record []string // filetime post: mark as "content seen by LLM" after execution
	Guard  []string // filetime pre: require "content seen by LLM" before execution
	Clear  []string // filetime post: invalidate "content seen" (e.g. after deletion)
}

// CommandSchema declares a plugin slash command's metadata.
type CommandSchema struct {
	Name        string // command name without "/" prefix (e.g. "deploy")
	Description string
	Hints       string // ghost text for TUI (e.g. "[environment]")
	Forward     bool   // if true, output sent to LLM as user message
	Silent      bool   // suppress command echo in chat history
}

// CommandResult is the return value from a plugin command execution.
type CommandResult struct {
	Output string // displayed to user (or forwarded to LLM if Forward=true)
}

// CompactionModification controls compaction behavior.
// Only processed for BeforeCompaction timing.
type CompactionModification struct {
	// Skip prevents built-in compaction from running — the plugin manages context itself.
	Skip bool
}

// BeforeCompactionContext is passed to BeforeCompaction hooks.
type BeforeCompactionContext struct {
	Context
	Forced         bool    // true if triggered by explicit /compact command or force=true
	TokensUsed     int     // estimated tokens before compaction
	ContextPercent float64 // percentage of context window currently used
	MessageCount   int     // messages in conversation
	KeepLast       int     // how many recent messages will be preserved
}

// MessageToolCall represents a tool call within a message (from LLM response).
// Distinct from ToolCall which represents a completed tool execution in history.
type MessageToolCall struct {
	ID        string         // Provider-generated call ID (e.g., "toolu_xxx", "call_xxx")
	Name      string         // Tool name
	Arguments map[string]any // Tool parameters
}

// Message represents a conversation message visible to transform plugins.
// IDs are positional (m0001, m0002, ...) and assigned during conversion.
type Message struct {
	ID         string            // Positional identifier (m0001, m0002, ...)
	Role       string            // user, assistant, system, tool
	Content    string            // Message text
	Thinking   string            // Thinking content (if any)
	ToolName   string            // Tool that produced this result (tool messages)
	ToolCallID string            // Links tool result to its call (tool messages)
	ToolCalls  []MessageToolCall // Tool calls in this message (assistant messages)
	Tokens     int               // Estimated token count
	Type       string            // normal, synthetic
	CreatedAt  time.Time         // Message timestamp
}

// TransformContext is passed to TransformMessages hooks.
type TransformContext struct {
	Context
	Messages []Message // ForLLM()-filtered messages with positional IDs
}
