package llamacpp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/adapter"
)

// deltaWithReasoning extends the streaming delta with reasoning_content (Qwen3, DeepSeek).
type deltaWithReasoning struct {
	ReasoningContent string `json:"reasoning_content"`
}

// Chat sends a Chat Completions request with streaming support.
// Overrides the inherited openai.Client.Chat() which uses Fantasy + Responses API.
// LlamaCPP uses the Chat Completions API directly via the OpenAI SDK, with
// support for reasoning_content (used by Qwen3, DeepSeek models).
func (c *Client) Chat(
	ctx context.Context,
	req request.Request,
	streamFunc stream.Func,
) (message.Message, usage.Usage, error) {
	params := toChatParams(req)

	s := c.Client.Client.Chat.Completions.NewStreaming(ctx, params)

	acc := openai.ChatCompletionAccumulator{}

	var (
		toolCalls []call.Call
		reasoning strings.Builder
	)

	for s.Next() {
		chunk := s.Current()
		acc.AddChunk(chunk)

		if tool, ok := acc.JustFinishedToolCall(); ok {
			tc, err := toolCallToCommon(tool)
			if err != nil {
				return message.Message{}, usage.Usage{}, err
			}

			toolCalls = append(toolCalls, tc)
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta

			// Extract reasoning_content from raw JSON (Qwen3, DeepSeek).
			if raw := delta.RawJSON(); raw != "" {
				var dr deltaWithReasoning
				if json.Unmarshal([]byte(raw), &dr) == nil && dr.ReasoningContent != "" {
					reasoning.WriteString(dr.ReasoningContent)

					if streamFunc != nil {
						if err := streamFunc(dr.ReasoningContent, "", false); err != nil {
							return message.Message{}, usage.Usage{}, err
						}
					}
				}
			}

			if delta.Content != "" && streamFunc != nil {
				if err := streamFunc("", delta.Content, false); err != nil {
					return message.Message{}, usage.Usage{}, err
				}
			}
		}
	}

	if err := s.Err(); err != nil {
		return message.Message{}, usage.Usage{}, adapter.MapError(err)
	}

	if streamFunc != nil {
		if err := streamFunc("", "", true); err != nil {
			return message.Message{}, usage.Usage{}, err
		}
	}

	var content string

	if len(acc.Choices) > 0 {
		content = acc.Choices[0].Message.Content
	}

	msg := message.Message{
		Role:     roles.Assistant,
		Content:  content,
		Thinking: reasoning.String(),
		Calls:    toolCalls,
	}

	u := usage.Usage{
		Input:  int(acc.Usage.PromptTokens),
		Output: int(acc.Usage.CompletionTokens),
	}

	return msg, u, nil
}

// toChatParams converts an aura request to OpenAI Chat Completions format.
func toChatParams(req request.Request) openai.ChatCompletionNewParams {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))

	for _, msg := range req.Messages {
		msgs = append(msgs, toAPIMessage(msg))
	}

	params := openai.ChatCompletionNewParams{
		Model:    req.Model.Name,
		Messages: msgs,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.NewOpt(true),
		},
	}

	if len(req.Tools) > 0 {
		tools := make([]openai.ChatCompletionToolUnionParam, len(req.Tools))
		for i, s := range req.Tools {
			tools[i] = openai.ChatCompletionToolUnionParam{
				OfFunction: &openai.ChatCompletionFunctionToolParam{
					Function: shared.FunctionDefinitionParam{
						Name:        s.Name,
						Description: param.NewOpt(s.Description),
						Parameters:  providers.BuildParametersMap(s.Parameters),
					},
				},
			}
		}

		params.Tools = tools
	}

	if req.Think != nil && req.Think.Bool() {
		params.ReasoningEffort = shared.ReasoningEffort(req.Think.String())
	}

	if g := req.Generation; g != nil {
		if g.Temperature != nil {
			params.Temperature = param.NewOpt(float64(*g.Temperature))
		}

		if g.TopP != nil {
			params.TopP = param.NewOpt(float64(*g.TopP))
		}

		if g.MaxOutputTokens != nil {
			params.MaxTokens = param.NewOpt(int64(*g.MaxOutputTokens))
		}

		if g.FrequencyPenalty != nil {
			params.FrequencyPenalty = param.NewOpt(float64(*g.FrequencyPenalty))
		}

		if g.PresencePenalty != nil {
			params.PresencePenalty = param.NewOpt(float64(*g.PresencePenalty))
		}

		if g.Stop != nil {
			params.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: g.Stop}
		}

		if g.Seed != nil {
			params.Seed = param.NewOpt(int64(*g.Seed))
		}
	}

	return params
}

// toAPIMessage converts an aura message to OpenAI Chat Completions format.
func toAPIMessage(msg message.Message) openai.ChatCompletionMessageParamUnion {
	switch msg.Role {
	case roles.System:
		return openai.SystemMessage(msg.Content)
	case roles.User:
		if len(msg.Images) > 0 {
			parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.Images)+1)
			if msg.Content != "" {
				parts = append(parts, openai.TextContentPart(msg.Content))
			}

			for _, img := range msg.Images {
				parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
					URL: img.DataURL(),
				}))
			}

			return openai.UserMessage(parts)
		}

		return openai.UserMessage(msg.Content)
	case roles.Assistant:
		if len(msg.Calls) > 0 {
			toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.Calls))
			for i, tc := range msg.Calls {
				argsJSON, _ := json.Marshal(tc.Arguments)

				toolCalls[i] = openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: string(argsJSON),
						},
					},
				}
			}

			return openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Content: openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: param.NewOpt(msg.Content),
					},
					ToolCalls: toolCalls,
				},
			}
		}

		return openai.AssistantMessage(msg.Content)
	case roles.Tool:
		return openai.ToolMessage(msg.ToolCallID, msg.Content)
	default:
		return openai.UserMessage(msg.Content)
	}
}

// toolCallToCommon converts an accumulated OpenAI tool call to aura's common format.
func toolCallToCommon(tc openai.FinishedChatCompletionToolCall) (call.Call, error) {
	var args map[string]any

	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return call.Call{}, err
		}
	}

	return call.Call{
		ID:        tc.ID,
		Name:      tc.Name,
		Arguments: args,
	}, nil
}
