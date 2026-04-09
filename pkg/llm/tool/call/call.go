// Package call defines the tool call type used in LLM messages.
package call

import (
	"fmt"
	"strings"

	"github.com/idelchi/aura/pkg/truncate"
)

// State represents the execution state of a tool call.
type State string

const (
	Pending  State = "pending"
	Running  State = "running"
	Complete State = "complete"
	Error    State = "error"
)

// Display length limits.
const (
	MaxArgsLen   = 400
	MaxResultLen = 400
)

// Call represents a tool invocation request from the LLM and its result.
type Call struct {
	// ID uniquely identifies this tool call (from LLM response).
	ID string `json:"id"`
	// Name is the tool function name.
	Name string `json:"name"`
	// Arguments contains the tool parameters (raw data for API calls).
	Arguments map[string]any `json:"arguments,omitempty"`
	// ArgumentsDisplay is the pre-formatted argument string for UI display.
	ArgumentsDisplay string `json:"-"`
	// Preview is the truncated tool result for UI display (not the full output).
	Preview string `json:"-"`
	// FullOutput stores the untruncated tool result for pager access.
	FullOutput string `json:"-"`
	// State indicates the current execution state.
	State State `json:"-"`
	// Error is set if State == Error.
	Error error `json:"-"`
	// ResultTokens is the rough token estimate of the full (untruncated) result.
	ResultTokens int `json:"-"`
}

// SetArgs formats and stores the argument display string from raw arguments.
func (c *Call) SetArgs(args map[string]any) {
	c.Arguments = args
	c.ArgumentsDisplay = truncate.Truncate(truncate.MapToJSON(args), MaxArgsLen)
}

// MarkRunning marks the call as currently executing.
func (c *Call) MarkRunning() {
	c.State = Running
}

// Complete marks the call as successfully completed with a truncated result.
// resultTokens is the pre-computed token estimate of the full result.
func (c *Call) Complete(result string, resultTokens int) {
	c.ResultTokens = resultTokens
	c.FullOutput = result
	c.Preview = truncate.Truncate(result, MaxResultLen)
	c.State = Complete
}

// Fail marks the call as failed with the given error.
// resultTokens is the pre-computed token estimate of the error message.
func (c *Call) Fail(err error, resultTokens int) {
	c.ResultTokens = resultTokens
	c.FullOutput = err.Error()
	c.Preview = truncate.Truncate(err.Error(), MaxResultLen)
	c.State = Error
	c.Error = err
}

// DisplayHeader returns the tool call header for UI display (e.g. "[Tool: name] args").
func (c Call) DisplayHeader() string {
	header := fmt.Sprintf("[Tool: %s]", c.Name)
	if c.ArgumentsDisplay != "" {
		header += " " + c.ArgumentsDisplay
	}

	return header
}

// DisplayFull returns the tool name header and arguments as separate strings,
// allowing callers to format them on separate lines.
func (c Call) DisplayFull() (header, args string) {
	header = fmt.Sprintf("[Tool: %s]", c.Name)
	if c.ArgumentsDisplay != "" {
		args = c.ArgumentsDisplay
	}

	return header, args
}

// DisplayResult returns the formatted result or error string for UI display.
func (c Call) DisplayResult() string {
	if c.State == Pending || c.State == Running {
		return ""
	}

	tokenInfo := fmt.Sprintf("[tokens: ~%d]", c.ResultTokens)

	if c.Error != nil {
		return fmt.Sprintf("%s\n✗ %v", tokenInfo, c.Error)
	}

	if c.Preview != "" {
		return fmt.Sprintf("%s\n→ %s", tokenInfo, strings.TrimSpace(c.Preview))
	}

	return tokenInfo + "\n→ (empty)"
}

// ForTranscript returns a compact representation for compaction transcripts.
func (c Call) ForTranscript() string {
	return fmt.Sprintf("  → %s: %v", c.Name, c.Arguments)
}

// ForLog returns a formatted string for log file output.
func (c Call) ForLog() string {
	return fmt.Sprintf("[assistant->%s] %v", c.Name, c.Arguments)
}
