package ollama

import (
	"context"
	"fmt"
	"strings"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// Chat sends a chat completion request with streaming support.
func (c *Client) Chat(
	ctx context.Context,
	req request.Request,
	streamFunc stream.Func,
) (message.Message, usage.Usage, error) {
	debug.Log("[ollama] Chat: model=%s contextLength=%d messages=%d tools=%d truncate=%v shift=%v",
		req.Model.Name, req.ContextLength, len(req.Messages), len(req.Tools), req.Truncate, req.Shift)

	if req.Generation != nil && req.Generation.ThinkBudget != nil {
		debug.Log("[ollama] think_budget=%d set but not supported by Ollama — ignoring", *req.Generation.ThinkBudget)
	}

	apiReq, err := c.ToChatRequest(req)
	if err != nil {
		return message.Message{}, usage.Usage{}, fmt.Errorf("building chat request: %w", err)
	}

	var (
		response                        api.ChatResponse
		thinkingBuilder, contentBuilder strings.Builder
		toolCalls                       []api.ToolCall
	)

	err = c.Client.Chat(ctx, apiReq, func(resp api.ChatResponse) error {
		response = resp

		// Accumulate thinking and content
		thinkingBuilder.WriteString(resp.Message.Thinking)
		contentBuilder.WriteString(resp.Message.Content)

		// Accumulate tool calls
		if len(resp.Message.ToolCalls) > 0 {
			toolCalls = append(toolCalls, resp.Message.ToolCalls...)
		}

		// Stream via callback if provided
		if streamFunc != nil {
			if err := streamFunc(
				resp.Message.Thinking,
				resp.Message.Content,
				false,
			); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		// Ollama's internal parser returns "error parsing tool call: raw=..." for malformed JSON.
		// Wrap with ErrToolCallParse so the assistant loop can detect and retry.
		if strings.Contains(err.Error(), "error parsing tool call") {
			return message.Message{}, usage.Usage{}, fmt.Errorf("%w: %w", tool.ErrToolCallParse, err)
		}

		return message.Message{}, usage.Usage{}, handleError(err)
	}

	// Send final done signal after streaming completes.
	if streamFunc != nil {
		if err := streamFunc("", "", true); err != nil {
			return message.Message{}, usage.Usage{}, err
		}
	}

	calls, err := ToCalls(toolCalls)
	if err != nil {
		return message.Message{}, usage.Usage{}, err
	}

	// Build the final message
	msg := message.Message{
		Role:     roles.Assistant,
		Content:  contentBuilder.String(),
		Thinking: thinkingBuilder.String(),
		Calls:    calls,
	}

	return msg, usage.Usage{
			Input: response.PromptEvalCount, Output: response.EvalCount,
		},
		nil
}
