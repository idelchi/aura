package assistant

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

func TestNormalizeThinkForModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		model       model.Model
		think       thinking.Value
		coerce      bool
		want        thinking.Value
		wantErr     bool
		wantErrText string
	}{
		{
			name:        "explicit effort rejected when model does not support thinking",
			model:       model.Model{Name: "plain"},
			think:       thinking.NewValue("high"),
			wantErr:     true,
			wantErrText: "does not support thinking",
		},
		{
			name:   "explicit effort coerced off when switching to non-thinking model",
			model:  model.Model{Name: "plain"},
			think:  thinking.NewValue("high"),
			coerce: true,
			want:   thinking.NewValue(false),
		},
		{
			name: "auto allowed when model supports thinking",
			model: model.Model{
				Name:         "reasoner",
				Capabilities: capabilities.Capabilities{capabilities.Thinking},
			},
			think: thinking.NewValue(true),
			want:  thinking.NewValue(true),
		},
		{
			name: "explicit effort rejected when levels are not supported",
			model: model.Model{
				Name:         "reasoner",
				Capabilities: capabilities.Capabilities{capabilities.Thinking},
			},
			think:       thinking.NewValue("high"),
			wantErr:     true,
			wantErrText: "not explicit thinking efforts",
		},
		{
			name: "explicit effort coerced to auto when levels are not supported",
			model: model.Model{
				Name:         "reasoner",
				Capabilities: capabilities.Capabilities{capabilities.Thinking},
			},
			think:  thinking.NewValue("high"),
			coerce: true,
			want:   thinking.NewValue(true),
		},
		{
			name: "unsupported model-specific effort rejected",
			model: model.Model{
				Name:             "bounded",
				Capabilities:     capabilities.Capabilities{capabilities.Thinking, capabilities.ThinkingLevels},
				ReasoningEfforts: []thinking.Effort{thinking.Low, thinking.Medium, thinking.High},
			},
			think:       thinking.NewValue("xhigh"),
			wantErr:     true,
			wantErrText: "supported: low, medium, high",
		},
		{
			name: "unsupported model-specific effort coerced to auto",
			model: model.Model{
				Name:             "bounded",
				Capabilities:     capabilities.Capabilities{capabilities.Thinking, capabilities.ThinkingLevels},
				ReasoningEfforts: []thinking.Effort{thinking.Low, thinking.Medium, thinking.High},
			},
			think:  thinking.NewValue("xhigh"),
			coerce: true,
			want:   thinking.NewValue(true),
		},
		{
			name: "supported model-specific effort allowed",
			model: model.Model{
				Name:             "deep",
				Capabilities:     capabilities.Capabilities{capabilities.Thinking, capabilities.ThinkingLevels},
				ReasoningEfforts: []thinking.Effort{thinking.High, thinking.XHigh},
			},
			think: thinking.NewValue("xhigh"),
			want:  thinking.NewValue("xhigh"),
		},
		{
			name:  "none allowed for non-thinking model",
			model: model.Model{Name: "plain"},
			think: thinking.NewValue("none"),
			want:  thinking.NewValue("none"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeThinkForModel(tt.model, tt.think, tt.coerce)
			if tt.wantErr {
				if err == nil {
					t.Fatal("normalizeThinkForModel() error = nil, want error")
				}

				if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("error = %q, want substring %q", err, tt.wantErrText)
				}

				return
			}

			if err != nil {
				t.Fatalf("normalizeThinkForModel() unexpected error: %v", err)
			}

			if got.Value != tt.want.Value {
				t.Fatalf("normalizeThinkForModel() = %#v, want %#v", got.Value, tt.want.Value)
			}
		})
	}
}
