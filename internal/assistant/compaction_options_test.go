package assistant

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/stream"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/llm/usage"
	"github.com/idelchi/aura/pkg/providers"
)

type compactionRequestProbe struct {
	providers.Provider
	chat func(request.Request) (message.Message, usage.Usage, error)
}

func (p compactionRequestProbe) Chat(_ context.Context, req request.Request, _ stream.Func) (message.Message, usage.Usage, error) {
	return p.chat(req)
}

// TestCompactionInferenceSettings exercises agent selection and both request paths.
// Self-use reads runtime settings; dedicated compaction never inherits the caller's controls.
func TestCompactionInferenceSettings(t *testing.T) {
	temperature, topP, penalty := 0.2, 0.9, 0.0
	topK, output, seed, budget := 20, 1024, 42, 512
	configured := &generation.Generation{
		Temperature: &temperature, TopP: &topP, TopK: &topK,
		FrequencyPenalty: &penalty, PresencePenalty: &penalty,
		MaxOutputTokens: &output, Stop: []string{"END SUMMARY"},
		Seed: &seed, ThinkBudget: &budget,
	}
	for _, tc := range []struct {
		name       string
		think      thinking.Value
		generation *generation.Generation
	}{
		{name: "configured", think: thinking.NewValue("low"), generation: configured},
		{name: "off", think: thinking.NewValue(false), generation: &generation.Generation{ThinkBudget: new(int)}},
		{name: "unset"},
	} {
		for _, dedicated := range []bool{false, true} {
			for _, chunks := range []int{1, 2} {
				t.Run(fmt.Sprintf("%s/dedicated=%t/chunks=%d", tc.name, dedicated, chunks), func(t *testing.T) {
					a := selectionAssistant(t)
					wanted := config.Model{
						Name: "compaction-model", Provider: "ollama", Context: 8192,
						Think: tc.think, Generation: tc.generation,
					}
					a.resolved.compactModel = &model.Model{Name: wanted.Name}
					a.cfg.Features.Compaction.Agent = "Disabled"
					if dedicated {
						a.cfg.Features.Compaction.Prompt = ""
						a.agent.Model.Think = thinking.NewValue("high")
						a.agent.Model.Generation = &generation.Generation{ThinkBudget: new(9999)}
						a.cfg.Agents.Apply(func(ag *config.Agent) {
							if ag.Name() == "Disabled" {
								ag.Metadata.Model = wanted
							}
						})
					} else {
						a.cfg.Features.Compaction.Prompt = "Compaction"
						a.agent.Model = wanted
					}
					resolved, err := a.ResolveCompaction(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					calls := 0
					resolved.provider = compactionRequestProbe{Provider: resolved.provider, chat: func(req request.Request) (message.Message, usage.Usage, error) {
						calls++
						if !reflect.DeepEqual(req.Think, wanted.Think.Ptr()) || !reflect.DeepEqual(req.Generation, wanted.Generation) {
							t.Errorf("request %d lost selected controls: think=%v generation=%+v", calls, req.Think, req.Generation)
						}
						if req.Model.Name != wanted.Name || req.ContextLength != wanted.Context {
							t.Errorf("wrong model/context: %s/%d", req.Model.Name, req.ContextLength)
						}
						if len(req.Tools) != 0 || !req.Truncate || !req.Shift || req.Messages[0].Content != resolved.prompt {
							t.Error("compaction request semantics changed")
						}
						return message.Message{Content: "Preserved facts."}, usage.Usage{}, nil
					}}
					history := message.Messages{{Role: roles.User, Content: "A"}, {Role: roles.Assistant, Content: "B"}}
					if chunks == 1 {
						_, err = a.CompactOnce(t.Context(), resolved, history, 200, true)
					} else {
						_, err = a.CompactChunks(t.Context(), resolved, history, chunks, 200)
					}
					if err != nil || calls != chunks {
						t.Fatalf("calls=%d, err=%v", calls, err)
					}
				})
			}
		}
	}

}

// TestCompactionRetryKeepsInferenceSettings covers the automatic overflow retry path.
func TestCompactionRetryKeepsInferenceSettings(t *testing.T) {
	a := selectionAssistant(t)
	a.cfg.Features.Compaction.Prompt = "Compaction"
	a.cfg.Features.Compaction.Chunks = 1
	a.cfg.Features.Compaction.TruncationRetries = []int{50}
	a.resolved.compactModel = &model.Model{Name: "test"}
	a.agent.Model.Think = thinking.NewValue("low")
	a.agent.Model.Generation = &generation.Generation{ThinkBudget: new(512)}
	a.builder.AddUserMessage(t.Context(), "Keep these facts.", 4)
	calls := 0
	a.agent.Provider = compactionRequestProbe{Provider: a.agent.Provider, chat: func(req request.Request) (message.Message, usage.Usage, error) {
		calls++
		if !reflect.DeepEqual(req.Think, thinking.NewValue("low").Ptr()) || req.Generation == nil || req.Generation.ThinkBudget == nil || *req.Generation.ThinkBudget != 512 {
			t.Errorf("retry %d lost inference settings", calls)
		}
		if calls == 1 {
			return message.Message{}, usage.Usage{}, providers.ErrContextExhausted
		}
		return message.Message{Content: "Preserved facts."}, usage.Usage{}, nil
	}}
	if err := a.CompactWith(t.Context(), true, 0); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("got %d calls, want initial attempt and retry", calls)
	}
}
