package openai

import (
	"encoding/json"

	"charm.land/fantasy"
	"github.com/openai/openai-go/v3"
)

// streamReasoning preserves the reasoning_content extension emitted by local
// OpenAI-compatible servers and gateways. Responses API reasoning is handled by Fantasy.
func streamReasoning(chunk openai.ChatCompletionChunk, yield func(fantasy.StreamPart) bool, state map[string]any) (map[string]any, bool) {
	if len(chunk.Choices) == 0 {
		return state, true
	}
	var delta struct {
		Content string `json:"reasoning_content"`
	}
	if err := json.Unmarshal([]byte(chunk.Choices[0].Delta.RawJSON()), &delta); err != nil || delta.Content == "" {
		return state, true
	}
	return state, yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: chunk.ID, Delta: delta.Content})
}
