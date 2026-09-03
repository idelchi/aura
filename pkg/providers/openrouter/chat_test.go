package openrouter

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/thinking"

	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
)

func TestBuildProviderOptionsThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		think      thinking.Value
		wantEffort *fantasyopenrouter.ReasoningEffort
		wantOpts   bool
	}{
		{name: "auto enables reasoning without effort", think: thinking.NewValue(true), wantOpts: true},
		{name: "off omits options", think: thinking.NewValue(false), wantOpts: false},
		{
			name:       "none maps through",
			think:      thinking.NewValue("none"),
			wantEffort: new(fantasyopenrouter.ReasoningEffortNone),
			wantOpts:   true,
		},
		{
			name:       "minimal maps through",
			think:      thinking.NewValue("minimal"),
			wantEffort: new(fantasyopenrouter.ReasoningEffortMinimal),
			wantOpts:   true,
		},
		{
			name:       "xhigh maps through",
			think:      thinking.NewValue("xhigh"),
			wantEffort: new(fantasyopenrouter.ReasoningEffortXHigh),
			wantOpts:   true,
		},
		{
			name:       "max maps through",
			think:      thinking.NewValue("max"),
			wantEffort: new(fantasyopenrouter.ReasoningEffort("max")),
			wantOpts:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := buildProviderOptions(request.Request{Think: tt.think.Ptr()})
			if !tt.wantOpts {
				if opts != nil {
					t.Fatalf("buildProviderOptions() = %#v, want nil", opts)
				}

				return
			}

			got, ok := opts[fantasyopenrouter.Name].(*fantasyopenrouter.ProviderOptions)
			if !ok {
				t.Fatalf("provider options type = %T, want *openrouter.ProviderOptions", opts[fantasyopenrouter.Name])
			}

			if got.Reasoning == nil {
				t.Fatal("Reasoning = nil, want non-nil")
			}

			if tt.wantEffort == nil {
				if got.Reasoning.Effort != nil {
					t.Fatalf("Effort = %q, want nil", *got.Reasoning.Effort)
				}

				return
			}

			if got.Reasoning.Effort == nil || *got.Reasoning.Effort != *tt.wantEffort {
				t.Fatalf("Effort = %v, want %v", got.Reasoning.Effort, tt.wantEffort)
			}
		})
	}
}
