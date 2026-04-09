// Package adapter bridges Aura's LLM types with Fantasy's unified provider abstraction.
// It converts requests, messages, tools, and streaming responses between the two systems.
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"

	"charm.land/fantasy"
)

// ToCall converts Aura messages and tool schemas to a Fantasy Call.
// System messages are converted to Fantasy system messages within the Prompt.
// Provider-specific options (thinking, reasoning, store) are NOT set here — each
// provider adds them via Call.ProviderOptions after calling ToCall.
func ToCall(msgs message.Messages, tools tool.Schemas) fantasy.Call {
	prompt := make(fantasy.Prompt, 0, len(msgs))

	for _, msg := range msgs {
		prompt = append(prompt, ToMessage(msg))
	}

	return fantasy.Call{
		Prompt: prompt,
		Tools:  ToTools(tools),
	}
}

// SetGeneration maps Aura's generation parameters onto a Fantasy Call.
func SetGeneration(c *fantasy.Call, gen *generation.Generation) {
	if gen == nil {
		return
	}

	if gen.Temperature != nil {
		c.Temperature = fantasy.Opt(*gen.Temperature)
	}

	if gen.TopP != nil {
		c.TopP = fantasy.Opt(*gen.TopP)
	}

	if gen.TopK != nil {
		c.TopK = fantasy.Opt(int64(*gen.TopK))
	}

	if gen.MaxOutputTokens != nil {
		c.MaxOutputTokens = fantasy.Opt(int64(*gen.MaxOutputTokens))
	}

	if gen.FrequencyPenalty != nil {
		c.FrequencyPenalty = fantasy.Opt(*gen.FrequencyPenalty)
	}

	if gen.PresencePenalty != nil {
		c.PresencePenalty = fantasy.Opt(*gen.PresencePenalty)
	}
}

// ToMessage converts a single Aura message to a Fantasy Message.
// For assistant messages, parts are ordered: ReasoningPart → TextPart → ToolCallPart(s).
func ToMessage(msg message.Message) fantasy.Message {
	var parts []fantasy.MessagePart

	switch msg.Role {
	case roles.System:
		parts = append(parts, fantasy.TextPart{Text: msg.Content})

	case roles.User:
		if msg.Content != "" {
			parts = append(parts, fantasy.TextPart{Text: msg.Content})
		}

		for _, img := range msg.Images {
			parts = append(parts, fantasy.FilePart{
				Data:      img.Data,
				MediaType: "image/jpeg",
			})
		}

	case roles.Assistant:
		if msg.Thinking != "" {
			parts = append(parts, fantasy.ReasoningPart{Text: msg.Thinking})
		}

		if msg.Content != "" {
			parts = append(parts, fantasy.TextPart{Text: msg.Content})
		}

		for _, c := range msg.Calls {
			argsJSON, _ := json.Marshal(c.Arguments)

			parts = append(parts, fantasy.ToolCallPart{
				ToolCallID: c.ID,
				ToolName:   c.Name,
				Input:      string(argsJSON),
			})
		}

	case roles.Tool:
		parts = append(parts, fantasy.ToolResultPart{
			ToolCallID: msg.ToolCallID,
			Output:     fantasy.ToolResultOutputContentText{Text: msg.Content},
		})

	default:
		if msg.Content != "" {
			parts = append(parts, fantasy.TextPart{Text: msg.Content})
		}
	}

	return fantasy.Message{
		Role:    fantasy.MessageRole(msg.Role),
		Content: parts,
	}
}

// ToTools converts Aura tool schemas to Fantasy FunctionTools.
func ToTools(schemas tool.Schemas) []fantasy.Tool {
	if len(schemas) == 0 {
		return nil
	}

	tools := make([]fantasy.Tool, len(schemas))

	for i, s := range schemas {
		tools[i] = fantasy.FunctionTool{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: providers.BuildParametersMap(s.Parameters),
		}
	}

	return tools
}

// StreamToMessage consumes a Fantasy stream, calls fn for incremental deltas,
// and returns the accumulated Aura message + usage.
// providerHint identifies the provider for ThinkingSignature extraction (e.g., "anthropic", "google").
func StreamToMessage(
	streamIter func(func(fantasy.StreamPart) bool),
	fn stream.Func,
	providerHint string,
) (message.Message, usage.Usage, error) {
	var (
		content   strings.Builder
		thinking  strings.Builder
		calls     []call.Call
		u         usage.Usage
		signature string
		toolInput strings.Builder
		streamErr error
	)

	for part := range streamIter {
		switch part.Type {
		case fantasy.StreamPartTypeTextDelta:
			content.WriteString(part.Delta)

			if fn != nil {
				if err := fn("", part.Delta, false); err != nil {
					return message.Message{}, usage.Usage{}, err
				}
			}

		case fantasy.StreamPartTypeReasoningDelta:
			thinking.WriteString(part.Delta)

			if fn != nil {
				if err := fn(part.Delta, "", false); err != nil {
					return message.Message{}, usage.Usage{}, err
				}
			}

			// Extract ThinkingSignature from provider metadata
			if providerHint != "" && part.ProviderMetadata != nil {
				if sig := extractSignature(part.ProviderMetadata, providerHint); sig != "" {
					signature = sig
				}
			}

		case fantasy.StreamPartTypeToolInputStart:
			toolInput.Reset()

		case fantasy.StreamPartTypeToolInputDelta:
			toolInput.WriteString(part.ToolCallInput)

		case fantasy.StreamPartTypeToolCall:
			input := part.ToolCallInput
			if input == "" {
				input = toolInput.String()
			}

			var args map[string]any

			if input != "" {
				if err := json.Unmarshal([]byte(input), &args); err != nil {
					return message.Message{}, usage.Usage{}, fmt.Errorf("%w: %w", tool.ErrToolCallParse, err)
				}
			}

			calls = append(calls, call.Call{
				ID:        part.ID,
				Name:      part.ToolCallName,
				Arguments: args,
			})

			toolInput.Reset()

		case fantasy.StreamPartTypeFinish:
			u = usage.Usage{
				Input:  int(part.Usage.InputTokens),
				Output: int(part.Usage.OutputTokens),
			}

		case fantasy.StreamPartTypeError:
			streamErr = part.Error
		}
	}

	if streamErr != nil {
		return message.Message{}, usage.Usage{}, MapError(streamErr)
	}

	// Done signal
	if fn != nil {
		if err := fn("", "", true); err != nil {
			return message.Message{}, usage.Usage{}, err
		}
	}

	msg := message.Message{
		Role:              roles.Assistant,
		Content:           content.String(),
		Thinking:          thinking.String(),
		ThinkingSignature: signature,
		Calls:             calls,
	}

	return msg, u, nil
}

// extractSignature pulls the ThinkingSignature from Fantasy's provider metadata.
// Each provider stores it differently — this tries the known formats.
// Since we can't import provider packages without creating a dependency,
// we use JSON round-trip to extract the Signature field generically.
func extractSignature(meta fantasy.ProviderMetadata, hint string) string {
	raw, ok := meta[hint]
	if !ok || raw == nil {
		return ""
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return ""
	}

	var fields struct {
		Signature string `json:"signature"`
	}

	if err := json.Unmarshal(data, &fields); err != nil {
		return ""
	}

	return fields.Signature
}

// MapError converts Fantasy errors to Aura error sentinels.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	var pe *fantasy.ProviderError
	if !errors.As(err, &pe) {
		if providers.IsNetworkError(err) {
			return providers.WrapNetworkError(err)
		}

		return err
	}

	if pe.IsContextTooLarge() {
		return providers.ErrContextExhausted
	}

	switch {
	case pe.StatusCode == http.StatusTooManyRequests:
		return &providers.RateLimitError{
			RetryAfter: parseRetryAfter(pe),
			Err:        fmt.Errorf("%s", pe.Message),
		}
	case pe.StatusCode == http.StatusUnauthorized || pe.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s", providers.ErrAuth, pe.Message)
	case pe.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", providers.ErrModelUnavailable, pe.Message)
	case pe.StatusCode >= 500:
		return fmt.Errorf("%w: %s", providers.ErrServerError, pe.Message)
	case pe.StatusCode == http.StatusPaymentRequired:
		return fmt.Errorf("%w: %s", providers.ErrCreditExhausted, pe.Message)
	default:
		if strings.Contains(pe.Message, "content") && strings.Contains(pe.Message, "filter") {
			return fmt.Errorf("%w: %s", providers.ErrContentFilter, pe.Message)
		}

		return fmt.Errorf("provider: %w", err)
	}
}

// parseRetryAfter extracts retry-after duration from a Fantasy ProviderError's headers.
func parseRetryAfter(pe *fantasy.ProviderError) time.Duration {
	if pe.ResponseHeaders == nil {
		return 0
	}

	// Standard Retry-After header (seconds)
	if ra, ok := pe.ResponseHeaders["retry-after"]; ok {
		if secs, err := strconv.Atoi(ra); err == nil {
			return time.Duration(secs) * time.Second
		}
	}

	// OpenAI uses retry-after-ms (milliseconds)
	if ms, ok := pe.ResponseHeaders["retry-after-ms"]; ok {
		if millis, err := strconv.Atoi(ms); err == nil {
			return time.Duration(millis) * time.Millisecond
		}
	}

	return 0
}
