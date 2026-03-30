// Package request defines the common chat request structure.
package request

import (
	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/responseformat"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/tool"
)

// Request represents a chat completion request.
type Request struct {
	// Model is the LLM model to use.
	Model model.Model
	// Think configures reasoning mode (nil=off, true="medium", "low"/"medium"/"high").
	Think *thinking.Value
	// Messages is the conversation history.
	Messages message.Messages
	// ContextLength is the maximum context window size.
	ContextLength int
	// Tools contains available tool definitions.
	Tools tool.Schemas
	// Truncate enables server-side truncation of input that exceeds the context window.
	// Used by compaction/feature agents where overflow is acceptable.
	Truncate bool
	// Shift enables server-side context window shifting on overflow.
	// Used by compaction/feature agents where tool call boundaries don't matter.
	Shift bool
	// Store controls whether the response is stored server-side.
	// nil = omit from request (default), false = required by codex provider.
	Store *bool
	// Generation holds optional sampling, output, and thinking budget parameters.
	// nil = no overrides (provider defaults apply).
	Generation *generation.Generation
	// ResponseFormat constrains the response to a specific format (nil = unconstrained text).
	ResponseFormat *responseformat.ResponseFormat
}
