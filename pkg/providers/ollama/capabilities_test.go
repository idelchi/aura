package ollama_test

import (
	"reflect"
	"testing"

	"github.com/ollama/ollama/api"
	ollamamodel "github.com/ollama/ollama/types/model"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/providers/ollama"
)

func TestWithCapabilitiesThinkingLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		family      string
		wantEfforts []thinking.Effort
	}{
		{
			name:   "most Ollama thinking models support max",
			family: "qwen3",
			wantEfforts: []thinking.Effort{
				thinking.Low,
				thinking.Medium,
				thinking.High,
				thinking.Max,
			},
		},
		{
			name:   "GPT-OSS stops at high",
			family: "gptoss",
			wantEfforts: []thinking.Effort{
				thinking.Low,
				thinking.Medium,
				thinking.High,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ollama.WithCapabilities(model.Model{Name: "thinking-model", Family: tt.family}, &api.ShowResponse{
				Capabilities: []ollamamodel.Capability{ollamamodel.CapabilityThinking},
			})

			if !got.Capabilities.Thinking() {
				t.Fatal("Thinking() = false, want true")
			}

			if !got.Capabilities.ThinkingLevels() {
				t.Fatal("ThinkingLevels() = false, want true")
			}

			if !reflect.DeepEqual(got.ReasoningEfforts, tt.wantEfforts) {
				t.Fatalf("ReasoningEfforts = %v, want %v", got.ReasoningEfforts, tt.wantEfforts)
			}
		})
	}
}

func TestWithCapabilitiesWithoutThinking(t *testing.T) {
	t.Parallel()

	got := ollama.WithCapabilities(model.Model{Name: "ordinary-model"}, &api.ShowResponse{})

	if got.Capabilities.Thinking() {
		t.Fatal("Thinking() = true, want false")
	}

	if got.Capabilities.ThinkingLevels() {
		t.Fatal("ThinkingLevels() = true, want false")
	}

	if len(got.ReasoningEfforts) != 0 {
		t.Fatalf("ReasoningEfforts = %v, want empty", got.ReasoningEfforts)
	}
}
