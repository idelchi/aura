package google

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/thinking"

	fantasygoogle "charm.land/fantasy/providers/google"
)

func TestBuildProviderOptionsThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		think     thinking.Value
		wantLevel *string
		wantOpts  bool
		wantErr   bool
	}{
		{name: "auto includes thoughts without level", think: thinking.NewValue(true), wantOpts: true},
		{name: "off omits options", think: thinking.NewValue(false), wantOpts: false},
		{name: "none omits options", think: thinking.NewValue("none"), wantOpts: false},
		{
			name:      "minimal maps through",
			think:     thinking.NewValue("minimal"),
			wantLevel: new(fantasygoogle.ThinkingLevelMinimal),
			wantOpts:  true,
		},
		{
			name:      "high maps through",
			think:     thinking.NewValue("high"),
			wantLevel: new(fantasygoogle.ThinkingLevelHigh),
			wantOpts:  true,
		},
		{name: "xhigh is rejected", think: thinking.NewValue("xhigh"), wantErr: true},
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

			got, ok := opts[fantasygoogle.Name].(*fantasygoogle.ProviderOptions)
			if !ok {
				t.Fatalf("provider options type = %T, want *google.ProviderOptions", opts[fantasygoogle.Name])
			}

			if got.ThinkingConfig == nil {
				t.Fatal("ThinkingConfig = nil, want non-nil")
			}

			if tt.wantLevel == nil {
				if got.ThinkingConfig.ThinkingLevel != nil {
					t.Fatalf("ThinkingLevel = %q, want nil", *got.ThinkingConfig.ThinkingLevel)
				}

				return
			}

			if got.ThinkingConfig.ThinkingLevel == nil || *got.ThinkingConfig.ThinkingLevel != *tt.wantLevel {
				t.Fatalf("ThinkingLevel = %v, want %v", got.ThinkingConfig.ThinkingLevel, tt.wantLevel)
			}
		})
	}
}
