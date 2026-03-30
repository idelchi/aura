package ollama

import (
	"encoding/json"
	"fmt"

	"github.com/ollama/ollama/api"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/responseformat"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
)

// ToChatRequest converts a common request to Ollama API format.
func (c *Client) ToChatRequest(request request.Request) (*api.ChatRequest, error) {
	model := request.Model

	shift := request.Shift
	truncate := request.Truncate

	chatReq := &api.ChatRequest{
		Model:    model.Name,
		Messages: ToAPIMessages(request.Messages),
		Tools:    ToTools(request.Tools),
		Options:  map[string]any{},
		Shift:    &shift,
		Truncate: &truncate,
	}

	// Handle thinking configuration — convert domain type to Ollama API type at the boundary.
	if request.Think != nil && request.Think.Bool() {
		chatReq.Think = &api.ThinkValue{Value: request.Think.Value}
	}

	// Handle context length
	if request.ContextLength > 0 {
		chatReq.Options["num_ctx"] = request.ContextLength
		debug.Log("[ollama] ToChatRequest: setting num_ctx=%d for model=%s", request.ContextLength, model.Name)
	} else {
		debug.Log("[ollama] ToChatRequest: no num_ctx override for model=%s (using Ollama default)", model.Name)
	}

	// Handle KeepAlive duration
	if c.keepAlive > 0 {
		chatReq.KeepAlive = &api.Duration{Duration: c.keepAlive}
	}

	// Generation parameters — set via Options map.
	// Ollama validates option types via reflection; use exact types (float32, int).
	if gen := request.Generation; gen != nil {
		if gen.Temperature != nil {
			chatReq.Options["temperature"] = float32(*gen.Temperature)
		}

		if gen.TopP != nil {
			chatReq.Options["top_p"] = float32(*gen.TopP)
		}

		if gen.TopK != nil {
			chatReq.Options["top_k"] = *gen.TopK
		}

		if gen.FrequencyPenalty != nil {
			chatReq.Options["frequency_penalty"] = float32(*gen.FrequencyPenalty)
		}

		if gen.PresencePenalty != nil {
			chatReq.Options["presence_penalty"] = float32(*gen.PresencePenalty)
		}

		if gen.MaxOutputTokens != nil {
			chatReq.Options["num_predict"] = *gen.MaxOutputTokens
		}

		if gen.Stop != nil {
			chatReq.Options["stop"] = gen.Stop
		}

		if gen.Seed != nil {
			chatReq.Options["seed"] = *gen.Seed
		}
	}

	// Response format.
	if request.ResponseFormat != nil {
		switch request.ResponseFormat.Type {
		case responseformat.JSONSchema:
			data, err := json.Marshal(request.ResponseFormat.Schema)
			if err != nil {
				return nil, fmt.Errorf("marshaling response format schema: %w", err)
			}

			chatReq.Format = data
		case responseformat.JSONObject:
			chatReq.Format = json.RawMessage(`"json"`)
		}
	}

	return chatReq, nil
}

// ToAPIMessage converts a common message to Ollama message format.
func ToAPIMessage(msg message.Message) api.Message {
	apiMsg := api.Message{
		Role:     msg.Role.String(),
		Content:  msg.Content,
		Thinking: msg.Thinking,
	}

	// Handle images for vision models
	if len(msg.Images) > 0 {
		images := make([]api.ImageData, 0, len(msg.Images))
		for _, img := range msg.Images {
			if len(img.Data) > 0 {
				images = append(images, api.ImageData(img.Data))
			}
		}

		apiMsg.Images = images
	}

	// Handle assistant messages with tool calls
	if msg.Role == roles.Assistant && len(msg.Calls) > 0 {
		apiMsg.ToolCalls = ToAPIToolCalls(msg.Calls)
	}

	// Handle tool result messages
	if msg.Role == roles.Tool {
		apiMsg.ToolName = msg.ToolName
		apiMsg.ToolCallID = msg.ToolCallID
	}

	return apiMsg
}

// ToAPIToolCalls converts common tool calls to Ollama format.
func ToAPIToolCalls(calls []call.Call) []api.ToolCall {
	result := make([]api.ToolCall, len(calls))
	for i, c := range calls {
		args := api.NewToolCallFunctionArguments()

		for k, v := range c.Arguments {
			args.Set(k, v)
		}

		result[i] = api.ToolCall{
			ID: c.ID,
			Function: api.ToolCallFunction{
				Name:      c.Name,
				Arguments: args,
			},
		}
	}

	return result
}

// ToAPIMessages converts common messages to Ollama format.
func ToAPIMessages(msgs message.Messages) []api.Message {
	result := make([]api.Message, len(msgs))
	for i, m := range msgs {
		result[i] = ToAPIMessage(m)
	}

	return result
}

// ToCall converts an Ollama tool call to common format.
func ToCall(tc api.ToolCall) (call.Call, error) {
	args := tc.Function.Arguments.ToMap()
	if args == nil {
		args = map[string]any{}
	}

	return call.Call{
		ID:        tc.ID,
		Name:      tc.Function.Name,
		Arguments: args,
	}, nil
}

// ToCalls converts Ollama tool calls to common format.
func ToCalls(tcs []api.ToolCall) ([]call.Call, error) {
	if len(tcs) == 0 {
		return nil, nil
	}

	result := make([]call.Call, len(tcs))

	for i, tc := range tcs {
		c, err := ToCall(tc)
		if err != nil {
			return nil, err
		}

		result[i] = c
	}

	return result, nil
}

// ToMessage converts an Ollama message to common format.
func ToMessage(msg api.Message) (message.Message, error) {
	calls, err := ToCalls(msg.ToolCalls)
	if err != nil {
		return message.Message{}, err
	}

	return message.Message{
		Role:     roles.Role(msg.Role),
		Content:  msg.Content,
		Thinking: msg.Thinking,
		Calls:    calls,
	}, nil
}
