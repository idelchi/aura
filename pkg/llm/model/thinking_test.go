package model

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

func TestNormalizeThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		model       Model
		think       thinking.Value
		coerce      bool
		want        thinking.Value
		wantErr     bool
		wantErrText string
	}{
		{
			name:  "unknown capabilities preserve explicit effort",
			model: Model{Name: "gateway/reasoner"},
			think: thinking.NewValue("medium"),
			want:  thinking.NewValue("medium"),
		},
		{
			name:   "switching to unknown capabilities preserves explicit effort",
			model:  Model{Name: "gateway/reasoner"},
			think:  thinking.NewValue("high"),
			coerce: true,
			want:   thinking.NewValue("high"),
		},
		{
			name:  "unknown capabilities preserve auto",
			model: Model{Name: "gateway/reasoner"},
			think: thinking.NewValue(true),
			want:  thinking.NewValue(true),
		},
		{
			name: "partial capabilities do not rule out explicit effort",
			model: Model{
				Name:         "gateway/reasoner",
				Capabilities: capabilities.Capabilities{capabilities.Thinking},
			},
			think: thinking.NewValue("low"),
			want:  thinking.NewValue("low"),
		},
		{
			name: "advertised efforts still constrain partial metadata",
			model: Model{
				Name:             "gateway/reasoner",
				ReasoningEfforts: []thinking.Effort{thinking.Low, thinking.Medium, thinking.High},
			},
			think:       thinking.NewValue("xhigh"),
			wantErr:     true,
			wantErrText: "supported: low, medium, high",
		},
		{
			name:        "explicit effort rejected when model does not support thinking",
			model:       Model{CapabilitiesKnown: true, Name: "plain"},
			think:       thinking.NewValue("high"),
			wantErr:     true,
			wantErrText: "does not support thinking",
		},
		{
			name:   "explicit effort coerced off when switching to non-thinking model",
			model:  Model{CapabilitiesKnown: true, Name: "plain"},
			think:  thinking.NewValue("high"),
			coerce: true,
			want:   thinking.NewValue(false),
		},
		{
			name: "auto allowed when model supports thinking",
			model: Model{
				CapabilitiesKnown: true,
				Name:              "reasoner",
				Capabilities:      capabilities.Capabilities{capabilities.Thinking},
			},
			think: thinking.NewValue(true),
			want:  thinking.NewValue(true),
		},
		{
			name: "explicit effort rejected when levels are not supported",
			model: Model{
				CapabilitiesKnown: true,
				Name:              "reasoner",
				Capabilities:      capabilities.Capabilities{capabilities.Thinking},
			},
			think:       thinking.NewValue("high"),
			wantErr:     true,
			wantErrText: "not explicit thinking efforts",
		},
		{
			name: "explicit effort coerced to auto when levels are not supported",
			model: Model{
				CapabilitiesKnown: true,
				Name:              "reasoner",
				Capabilities:      capabilities.Capabilities{capabilities.Thinking},
			},
			think:  thinking.NewValue("high"),
			coerce: true,
			want:   thinking.NewValue(true),
		},
		{
			name: "unsupported model-specific effort rejected",
			model: Model{
				CapabilitiesKnown: true,
				Name:              "bounded",
				Capabilities:      capabilities.Capabilities{capabilities.Thinking, capabilities.ThinkingLevels},
				ReasoningEfforts:  []thinking.Effort{thinking.Low, thinking.Medium, thinking.High},
			},
			think:       thinking.NewValue("xhigh"),
			wantErr:     true,
			wantErrText: "supported: low, medium, high",
		},
		{
			name: "unsupported model-specific effort coerced to auto",
			model: Model{
				CapabilitiesKnown: true,
				Name:              "bounded",
				Capabilities:      capabilities.Capabilities{capabilities.Thinking, capabilities.ThinkingLevels},
				ReasoningEfforts:  []thinking.Effort{thinking.Low, thinking.Medium, thinking.High},
			},
			think:  thinking.NewValue("xhigh"),
			coerce: true,
			want:   thinking.NewValue(true),
		},
		{
			name: "supported model-specific effort allowed",
			model: Model{
				CapabilitiesKnown: true,
				Name:              "deep",
				Capabilities:      capabilities.Capabilities{capabilities.Thinking, capabilities.ThinkingLevels},
				ReasoningEfforts:  []thinking.Effort{thinking.High, thinking.XHigh},
			},
			think: thinking.NewValue("xhigh"),
			want:  thinking.NewValue("xhigh"),
		},
		{
			name:  "none allowed for non-thinking model",
			model: Model{CapabilitiesKnown: true, Name: "plain"},
			think: thinking.NewValue("none"),
			want:  thinking.NewValue("none"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.model.NormalizeThinking(tt.think, tt.coerce)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NormalizeThinking() error = nil, want error")
				}

				if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("error = %q, want substring %q", err, tt.wantErrText)
				}

				return
			}

			if err != nil {
				t.Fatalf("NormalizeThinking() unexpected error: %v", err)
			}

			if got.Value != tt.want.Value {
				t.Fatalf("NormalizeThinking() = %#v, want %#v", got.Value, tt.want.Value)
			}
		})
	}
}
