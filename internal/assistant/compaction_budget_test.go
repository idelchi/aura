package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/injector"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

// compactionProbe replaces only Chat; configuration/model selection use Aura's
// real assistant and cached model resolution.
type compactionProbe struct {
	providers.Provider                                                             // other provider methods are unchanged
	chat               func(context.Context) (message.Message, usage.Usage, error) // deterministic response
}

// Chat returns the controlled response without contacting an inference service.
func (p compactionProbe) Chat(ctx context.Context, _ request.Request, _ stream.Func) (message.Message, usage.Usage, error) {
	return p.chat(ctx)
}

// TestCompactionFailureBudgets covers invalid output, time limits and the outer
// recovery loop: neither should multiply into nested transcript retries.
func TestCompactionFailureBudgets(t *testing.T) {
	for _, timed := range []bool{false, true} {
		t.Run(map[bool]string{false: "tool call", true: "deadline"}[timed], func(t *testing.T) {
			a := selectionAssistant(t)
			a.cfg.Features.Compaction.Prompt = "Compaction"
			a.cfg.Features.Compaction.Chunks = 1
			a.cfg.Features.Compaction.Timeout = 50 * time.Millisecond
			a.cfg.Features.Compaction.TruncationRetries = []int{150, 100, 50, 0}
			a.resolved.compactModel = &model.Model{Name: "test"}
			a.builder.AddUserMessage(t.Context(), "Assess these logs", 4)
			calls := 0
			a.agent.Provider = compactionProbe{Provider: a.agent.Provider, chat: func(ctx context.Context) (message.Message, usage.Usage, error) {
				calls++
				if timed {
					<-ctx.Done()
					return message.Message{}, usage.Usage{}, ctx.Err()
				}
				return message.Message{Calls: []call.Call{{ID: "invalid", Name: "Gotify"}}}, usage.Usage{}, nil
			}}
			err := a.RecoverCompaction(t.Context(), 0)
			if calls != 1 || err == nil {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if timed && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lost deadline: %v", err)
			}
			if !timed && !errors.Is(err, ErrCompactionExhausted) {
				t.Fatalf("lost compaction failure: %v", err)
			}
		})
	}
}

// TestCompactionReceipts keeps real acknowledgements outside model-authored prose.
func TestCompactionReceipts(t *testing.T) {
	got := compactionReceipts([]injector.ToolCall{{Name: "Gotify", Result: "Notification suppressed [INFO]: task (minimum level: ERROR)"}, {Name: "logs", Result: strings.Repeat("x", 500)}}, 0)
	if !strings.Contains(got, "Notification suppressed") || strings.Contains(got, "Notification sent") || !strings.Contains(got, "response excerpt") {
		t.Fatal(got)
	}
	if !strings.Contains(compactionReceipts(nil, 200), "No tools were invoked") {
		t.Fatal("missing no-delivery evidence")
	}
}
