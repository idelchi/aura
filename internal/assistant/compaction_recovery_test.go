package assistant

import (
	"context"
	"errors"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/providers"
)

// TestSplitHistoryZero compacts the entire non-system history, including the last tool batch.
func TestSplitHistoryZero(t *testing.T) {
	t.Parallel()
	history := message.Messages{
		{Role: roles.System, Content: "system"},
		{Role: roles.User, Content: "inspect"},
		{Role: roles.Assistant, Calls: []call.Call{{ID: "one", Name: "Read"}}},
		{Role: roles.Tool, ToolCallID: "one", Content: "large result"},
	}
	compact, preserved := splitHistory(history, 0)
	if len(compact) != 3 || len(preserved) != 0 {
		t.Fatalf("compact=%d preserved=%d", len(compact), len(preserved))
	}
	compact, preserved = splitHistory(history, 1)
	if len(compact) != 1 || len(preserved) != 2 {
		t.Fatalf("split damaged tool batch: compact=%d preserved=%d", len(compact), len(preserved))
	}
}

// TestOverflowRetryLimit terminates without starting another compaction or losing the provider cause.
func TestOverflowRetryLimit(t *testing.T) {
	t.Parallel()
	a := &Assistant{loop: loopState{overflowRecoveries: maxContextRecoveries}}
	var parse, retry int
	result, err := a.handleChatError(t.Context(), providers.ErrContextExhausted, nil, &parse, &retry)
	if result != chatFatal || !errors.Is(err, ErrCompactionExhausted) || !errors.Is(err, providers.ErrContextExhausted) {
		t.Fatalf("outcome=%v error=%v", result, err)
	}
}

// TestRecoveryCancellation does not make compaction requests after cancellation.
func TestRecoveryCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (&Assistant{}).RecoverCompaction(ctx, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}
