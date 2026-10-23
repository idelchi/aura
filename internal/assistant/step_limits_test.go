package assistant

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/tool/call"
	"github.com/idelchi/aura/pkg/llm/usage"
)

// Exercise the real tool loop, including its per-turn configuration reload.
func TestStepLimits(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		limit, finishAt, wantCalls int
		cancel                     bool
		wantErr                    error
	}{
		{name: "positive cap", limit: 2, finishAt: 61, wantCalls: 3, wantErr: ErrMaxSteps},
		{name: "unlimited exceeds default", limit: -1, finishAt: 61, wantCalls: 61},
		{name: "unlimited preserves cancellation", limit: -1, finishAt: 3, wantCalls: 3, cancel: true, wantErr: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := selectionAssistant(t)
			path := filepath.Join(a.configOpts.WriteHome, "config/features/tools.yaml")
			if err := os.WriteFile(path, fmt.Appendf(nil, "tools:\n  max_steps: %d\n  token_budget: 0\n", tc.limit), 0o600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(_ request.Request) (message.Message, usage.Usage, error) {
				calls++
				if calls == tc.finishAt {
					if tc.cancel {
						return message.Message{}, usage.Usage{}, context.Canceled
					}
					return message.Message{Content: "Completed."}, usage.Usage{}, nil
				}
				return message.Message{Calls: []call.Call{{ID: fmt.Sprint(calls), Name: "SelectionProbe", Arguments: map[string]any{}}}}, usage.Usage{}, nil
			}}
			err := a.ProcessInput(t.Context(), "Execute the requested tool sequence.")
			if !errors.Is(err, tc.wantErr) || calls != tc.wantCalls {
				t.Fatalf("calls=%d, error=%v; want calls=%d, error=%v", calls, err, tc.wantCalls, tc.wantErr)
			}
		})
	}
}
