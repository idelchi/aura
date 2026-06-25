package anthropic

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/thinking"

	fanthropic "charm.land/fantasy/providers/anthropic"
)

func TestBuildProviderOptionsThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		think      thinking.Value
		wantEffort *fanthropic.Effort
		wantOpts   bool
		wantErr    bool
	}{
		{
			name:       "auto maps to high adaptive effort",
			think:      thinking.NewValue(true),
			wantEffort: anthropicEffortPtr(fanthropic.EffortHigh),
			wantOpts:   true,
		},
		{name: "off omits options", think: thinking.NewValue(false), wantOpts: false},
		{name: "none omits options", think: thinking.NewValue("none"), wantOpts: false},
		{
			name:       "xhigh maps through",
			think:      thinking.NewValue("xhigh"),
			wantEffort: anthropicEffortPtr(fanthropic.Effort("xhigh")),
			wantOpts:   true,
		},
		{
			name:       "max maps through",
			think:      thinking.NewValue("max"),
			wantEffort: anthropicEffortPtr(fanthropic.EffortMax),
			wantOpts:   true,
		},
		{name: "minimal is rejected", think: thinking.NewValue("minimal"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts, err := buildProviderOptions(request.Request{Think: tt.think.Ptr()})
			if tt.wantErr {
				if err == nil {
					t.Fatal("buildProviderOptions() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("buildProviderOptions() unexpected error: %v", err)
			}

			if !tt.wantOpts {
				if len(opts) != 0 {
					t.Fatalf("buildProviderOptions() = %#v, want empty", opts)
				}

				return
			}

			got, ok := opts[fanthropic.Name].(*fanthropic.ProviderOptions)
			if !ok {
				t.Fatalf("provider options type = %T, want *anthropic.ProviderOptions", opts[fanthropic.Name])
			}

			if got.Effort == nil || *got.Effort != *tt.wantEffort {
				t.Fatalf("Effort = %v, want %v", got.Effort, tt.wantEffort)
			}
		})
	}
}

func anthropicEffortPtr(e fanthropic.Effort) *fanthropic.Effort {
	return &e
}
