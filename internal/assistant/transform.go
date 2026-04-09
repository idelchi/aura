package assistant

import (
	"fmt"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/sdk"
)

// toSDKMessages converts ForLLM-filtered messages to SDK Message types with positional IDs.
// Positional IDs (m0001, m0002, ...) are stable within a single transform call.
func toSDKMessages(msgs message.Messages) []sdk.Message {
	result := make([]sdk.Message, len(msgs))

	for i, msg := range msgs {
		sdkMsg := sdk.Message{
			ID:         fmt.Sprintf("m%04d", i+1),
			Role:       string(msg.Role),
			Content:    msg.Content,
			Thinking:   msg.Thinking,
			ToolName:   msg.ToolName,
			ToolCallID: msg.ToolCallID,
			Tokens:     msg.Tokens.Total,
			Type:       messageTypeString(msg.Type),
			CreatedAt:  msg.CreatedAt,
		}

		if len(msg.Calls) > 0 {
			sdkMsg.ToolCalls = make([]sdk.MessageToolCall, len(msg.Calls))
			for j, c := range msg.Calls {
				args := c.Arguments
				if args == nil {
					args = map[string]any{}
				}

				sdkMsg.ToolCalls[j] = sdk.MessageToolCall{
					ID:        c.ID,
					Name:      c.Name,
					Arguments: args,
				}
			}
		}

		result[i] = sdkMsg
	}

	return result
}

// applyTransform merges transformed SDK messages back with original messages.
// For messages with matching positional IDs: clones the original, overlays Content/Thinking.
// For new messages (no matching ID): constructs minimal message.Message from SDK fields.
// Preserves ThinkingSignature, Images, call.Call details from originals.
func applyTransform(originals message.Messages, transformed []sdk.Message) message.Messages {
	// Build positional ID → index lookup for originals.
	idxByID := make(map[string]int, len(originals))
	for i := range originals {
		idxByID[fmt.Sprintf("m%04d", i+1)] = i
	}

	result := make(message.Messages, 0, len(transformed))

	for _, sdkMsg := range transformed {
		idx, found := idxByID[sdkMsg.ID]
		if found {
			// Clone original, overlay Content/Thinking if changed.
			orig := originals[idx]

			if sdkMsg.Content != orig.Content {
				orig.Content = sdkMsg.Content
			}

			if sdkMsg.Thinking != orig.Thinking {
				orig.Thinking = sdkMsg.Thinking
			}

			result = append(result, orig)
		} else {
			// New message (e.g., compression summary) — construct minimal.
			result = append(result, message.Message{
				Role:    roles.Role(sdkMsg.Role),
				Content: sdkMsg.Content,
				Type:    message.Normal,
			})
		}
	}

	return result
}

// messageTypeString converts a message.Type to a string for SDK consumption.
func messageTypeString(t message.Type) string {
	switch t {
	case message.Normal:
		return "normal"
	case message.Synthetic:
		return "synthetic"
	case message.Ephemeral:
		return "ephemeral"
	case message.DisplayOnly:
		return "display_only"
	case message.Bookmark:
		return "bookmark"
	case message.Metadata:
		return "metadata"
	default:
		return "normal"
	}
}
