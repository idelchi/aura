package adapter_test

import (
	"testing"

	"charm.land/fantasy"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/providers/adapter"
)

// TestStreamReportsFinishReason carries the finish part's reason onto the message for
// every provider built on the Fantasy stream (OpenAI, Anthropic, Google, OpenRouter, Codex).
func TestStreamReportsFinishReason(t *testing.T) {
	for reason, want := range map[fantasy.FinishReason]message.Finish{
		fantasy.FinishReasonStop:          message.FinishStop,
		fantasy.FinishReasonLength:        message.FinishLength,
		fantasy.FinishReasonToolCalls:     message.FinishToolCalls,
		fantasy.FinishReasonContentFilter: message.FinishContentFilter,
		fantasy.FinishReasonError:         message.FinishError,
		fantasy.FinishReasonOther:         message.FinishOther,
		fantasy.FinishReasonUnknown:       message.FinishUnreported,
	} {
		parts := []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextDelta, Delta: "partial"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: reason, Usage: fantasy.Usage{InputTokens: 3, OutputTokens: 2}},
		}

		msg, used, err := adapter.StreamToMessage(func(yield func(fantasy.StreamPart) bool) {
			for _, part := range parts {
				if !yield(part) {
					return
				}
			}
		}, nil, "")
		if err != nil || msg.Content != "partial" || msg.Finish != want || used.Output != 2 {
			t.Fatalf("%q: content=%q finish=%q output=%d err=%v, want %q", reason, msg.Content, msg.Finish, used.Output, err, want)
		}
	}

	msg, _, err := adapter.StreamToMessage(func(func(fantasy.StreamPart) bool) {}, nil, "")
	if err != nil || msg.Finish != message.FinishUnreported {
		t.Fatalf("a stream without a finish part must stay unreported: %q %v", msg.Finish, err)
	}
}
