package codex

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/thinking"

	fantasyopenai "charm.land/fantasy/providers/openai"
)

func TestReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		think      thinking.Value
		wantEffort fantasyopenai.ReasoningEffort
		wantOK     bool
		wantErr    bool
	}{
		{name: "auto omits effort", think: thinking.NewValue(true)},
		{
			name:       "off maps to none",
			think:      thinking.NewValue(false),
			wantEffort: fantasyopenai.ReasoningEffortNone,
			wantOK:     true,
		},
		{
			name:       "minimal maps through",
			think:      thinking.NewValue("minimal"),
			wantEffort: fantasyopenai.ReasoningEffortMinimal,
			wantOK:     true,
		},
		{
			name:       "xhigh maps through",
			think:      thinking.NewValue("xhigh"),
			wantEffort: fantasyopenai.ReasoningEffortXHigh,
			wantOK:     true,
		},
		{name: "max is rejected", think: thinking.NewValue("max"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := reasoningEffort(tt.think.Ptr())
			if tt.wantErr {
				if err == nil {
					t.Fatal("reasoningEffort() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("reasoningEffort() unexpected error: %v", err)
			}

			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}

			if ok && got != tt.wantEffort {
				t.Fatalf("effort = %q, want %q", got, tt.wantEffort)
			}
		})
	}
}
