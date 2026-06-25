package openai

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/thinking"

	fantasyopenai "charm.land/fantasy/providers/openai"
)

func TestBuildProviderOptionsThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		think      thinking.Value
		wantEffort *fantasyopenai.ReasoningEffort
		wantOpts   bool
		wantErr    bool
	}{
		{name: "auto omits effort", think: thinking.NewValue(true), wantOpts: false},
		{
			name:       "off maps to none",
			think:      thinking.NewValue(false),
			wantEffort: effortPtr(fantasyopenai.ReasoningEffortNone),
			wantOpts:   true,
		},
		{
			name:       "xhigh maps through",
			think:      thinking.NewValue("xhigh"),
			wantEffort: effortPtr(fantasyopenai.ReasoningEffortXHigh),
			wantOpts:   true,
		},
		{name: "max is rejected", think: thinking.NewValue("max"), wantErr: true},
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

			got, ok := opts[fantasyopenai.Name].(*fantasyopenai.ProviderOptions)
			if !ok {
				t.Fatalf("provider options type = %T, want *openai.ProviderOptions", opts[fantasyopenai.Name])
			}

			if got.ReasoningEffort == nil || *got.ReasoningEffort != *tt.wantEffort {
				t.Fatalf("ReasoningEffort = %v, want %v", got.ReasoningEffort, tt.wantEffort)
			}
		})
	}
}

func effortPtr(e fantasyopenai.ReasoningEffort) *fantasyopenai.ReasoningEffort {
	return &e
}
