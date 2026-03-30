package config

import "github.com/idelchi/aura/pkg/llm/tool"

// ReadBeforeConfig holds YAML-level read-before policy settings.
// Uses *bool so nil = inherit default via mergo merge chain.
type ReadBeforeConfig struct {
	Write  *bool `yaml:"write"`  // nil = default (true)
	Delete *bool `yaml:"delete"` // nil = default (false)
}

// ToPolicy converts the config to a resolved ReadBeforePolicy,
// applying defaults for nil fields.
func (r ReadBeforeConfig) ToPolicy() tool.ReadBeforePolicy {
	p := tool.DefaultReadBeforePolicy()

	if r.Write != nil {
		p.Write = *r.Write
	}

	if r.Delete != nil {
		p.Delete = *r.Delete
	}

	return p
}

// ToolBash groups Bash tool configuration (truncation + command rewrite).
type ToolBash struct {
	// Truncation controls middle-truncation of Bash tool output.
	Truncation BashTruncation `yaml:"truncation"`
	// Rewrite is a Go text/template applied to every Bash tool command before execution.
	// Template receives {{ .Command }} (the original command) and sprig functions.
	// Empty = no rewrite. Example: "rtk {{ .Command }}"
	Rewrite string `yaml:"rewrite"`
}

// ToolResult groups tool result size guard configuration.
type ToolResult struct {
	// MaxTokens is the estimated token limit for a single tool result (used when mode = "tokens").
	MaxTokens int `yaml:"max_tokens"`
	// MaxPercentage is the context-fill percentage above which results are rejected (used when mode = "percentage").
	MaxPercentage float64 `yaml:"max_percentage"`
}

// ToolExecution holds configuration for tool execution guards.
type ToolExecution struct {
	// Mode selects the guard strategy: "tokens" (fixed limit) or "percentage" (context-fill based).
	Mode string `validate:"omitempty,oneof=percentage tokens" yaml:"mode"`
	// Result groups tool result size guard fields.
	Result ToolResult `yaml:"result"`
	// ReadSmallFileTokens is the token threshold below which the Read tool ignores line ranges and returns the full
	// file.
	ReadSmallFileTokens int `yaml:"read_small_file_tokens"`
	// MaxSteps caps the total number of iterations to prevent runaway loops.
	// At this limit, tools are disabled and the LLM must respond with text only.
	MaxSteps int `yaml:"max_steps"`
	// TokenBudget is the cumulative token limit (input + output) for a session.
	// Once reached, the assistant stops immediately. 0 = disabled.
	TokenBudget int `yaml:"token_budget"`
	// RejectionMessage is the fmt template sent back as the tool result when rejected.
	// Receives two %d arguments: estimated tokens, limit.
	RejectionMessage string `yaml:"rejection_message"`
	// Bash groups Bash tool configuration (truncation + command rewrite).
	Bash ToolBash `yaml:"bash"`
	// UserInputMaxPercentage is the context-fill percentage above which user input messages are rejected.
	// Prevents massive messages (typed, @Bash, @File) from entering the conversation and causing
	// unrecoverable context exhaustion. 0 = disabled.
	UserInputMaxPercentage float64 `yaml:"user_input_max_percentage"`
	// ReadBefore controls which file operations require a prior read.
	// write: true (default) = must read before overwriting existing files.
	// delete: false (default) = no read required before deleting.
	// Toggled at runtime with /readbefore (alias /rb).
	ReadBefore ReadBeforeConfig `yaml:"read_before"`
	// Enabled is the default for --include-tools. Glob patterns for tools to include.
	// Empty = no include filter (all tools pass). CLI --include-tools overrides this.
	Enabled []string `yaml:"enabled"`
	// Disabled is the default for --exclude-tools. Glob patterns for tools to exclude.
	// Empty = no exclude filter. CLI --exclude-tools overrides this.
	Disabled []string `yaml:"disabled"`
	// OptIn lists tool names that are registered but hidden unless explicitly enabled
	// by name or narrow glob at any layer (CLI, features, agent, mode, or task).
	// The bare "*" wildcard does NOT satisfy opt-in — only explicit names or narrow globs do.
	OptIn []string `yaml:"opt_in"`
	// Deferred lists tool name patterns excluded from request.Tools and listed in a
	// lightweight system prompt index instead. The model can load them on demand via
	// the LoadTools meta-tool. Empty = all tools are eager (no change in behavior).
	Deferred []string `yaml:"deferred"`
	// WebFetchMaxBodySize is the maximum response body size in bytes for the WebFetch tool.
	WebFetchMaxBodySize int64 `yaml:"webfetch_max_body_size"`
	// Parallel enables concurrent execution of independent tool calls within a single LLM turn.
	// nil = true (default). Explicit false = sequential.
	Parallel *bool `yaml:"parallel"`
	// Policy controls which tool calls are auto-approved, require confirmation, or are blocked.
	// Merged via the standard features override chain (global → agent → mode).
	// Approval rules from config/rules/ are merged additively on top.
	Policy ToolPolicy `yaml:"policy"`
}

// ParallelEnabled returns whether parallel tool execution is active.
// nil (default) = true.
func (t ToolExecution) ParallelEnabled() bool {
	return t.Parallel == nil || *t.Parallel
}

// BashTruncation holds configuration for Bash tool output truncation.
// Output exceeding MaxLines is middle-truncated: first HeadLines + last TailLines,
// with full output saved to a temp file.
// Pointer fields distinguish "not set" (nil → apply default) from explicit zero
// (0 → truncate all output, exit-code only).
type BashTruncation struct {
	// MaxOutputBytes caps the number of bytes captured from stdout/stderr during execution.
	// Prevents OOM from unbounded output (e.g., binary dumps, base64).
	// nil = apply default (1MB), 0 = disabled (no byte cap), N = cap at N bytes.
	MaxOutputBytes *int `yaml:"max_output_bytes"`
	// MaxLines is the line count threshold above which output is truncated.
	// nil = disabled (no truncation), 0 = exit-code only, N = truncate after N lines.
	MaxLines *int `yaml:"max_lines"`
	// HeadLines is the number of lines kept from the beginning.
	HeadLines *int `yaml:"head_lines"`
	// TailLines is the number of lines kept from the end.
	TailLines *int `yaml:"tail_lines"`
}

// ApplyDefaults sets sane defaults for zero-valued fields.
func (t *ToolExecution) ApplyDefaults() error {
	if t.Mode == "" {
		t.Mode = "percentage"
	}

	if t.Result.MaxTokens == 0 {
		t.Result.MaxTokens = 20000
	}

	if t.Result.MaxPercentage == 0 {
		t.Result.MaxPercentage = 95
	}

	if t.ReadSmallFileTokens == 0 {
		t.ReadSmallFileTokens = 2000
	}

	if t.MaxSteps == 0 {
		t.MaxSteps = 50
	}

	if t.RejectionMessage == "" {
		t.RejectionMessage = "Error: Tool result too large (%d tokens, limit %d). Try a more specific query or use offset/limit parameters."
	}

	if t.Bash.Truncation.MaxOutputBytes == nil {
		v := 1048576 // 1MB

		t.Bash.Truncation.MaxOutputBytes = &v
	}

	if t.Bash.Truncation.MaxLines == nil {
		v := 200

		t.Bash.Truncation.MaxLines = &v
	}

	if t.Bash.Truncation.HeadLines == nil {
		v := 100

		t.Bash.Truncation.HeadLines = &v
	}

	if t.Bash.Truncation.TailLines == nil {
		v := 80

		t.Bash.Truncation.TailLines = &v
	}

	if t.UserInputMaxPercentage == 0 {
		t.UserInputMaxPercentage = 80
	}

	if t.WebFetchMaxBodySize == 0 {
		t.WebFetchMaxBodySize = 5 * 1024 * 1024
	}

	return nil
}
