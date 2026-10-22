package assistant

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/providers"
	"github.com/idelchi/aura/pkg/llm/generation"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/thinking"
)

// Exercise selected helper controls at the HTTP boundary, for both self-use and
// dedicated agents. Dedicated helpers must not inherit their caller's settings.
func TestFeatureInferenceSettings(t *testing.T) {
	for _, tc := range []struct {
		name       string
		think      thinking.Value
		generation *generation.Generation
	}{
		{name: "configured", think: thinking.NewValue("low"), generation: &generation.Generation{
			Temperature: new(0.2), TopK: new(20), MaxOutputTokens: new(64), ThinkBudget: new(32),
		}},
		{name: "off", think: thinking.NewValue(false)},
		{name: "unset"},
	} {
		for _, dedicated := range []bool{false, true} {
			for _, feature := range []string{"tool_calls", "user_messages", "title", "thinking"} {
				t.Run(fmt.Sprintf("%s/dedicated=%t/%s", tc.name, dedicated, feature), func(t *testing.T) {
					requests := make(chan map[string]any, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/v1/chat/completions" {
							t.Errorf("unexpected endpoint: %s", r.URL.Path)
							http.NotFound(w, r)
							return
						}
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						requests <- body
						chunk := map[string]any{"id": "test", "object": "chat.completion.chunk", "choices": []any{
							map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": `{"result":"safe"}`}, "finish_reason": "stop"},
						}}
						data, err := json.Marshal(chunk)
						if err != nil {
							t.Error(err)
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
					}))
					defer server.Close()
					a := selectionAssistant(t)
					provider := a.cfg.Providers.Get("ollama")
					provider.Type, provider.URL = "llamacpp", server.URL
					provider.Cache = nil
					a.cfg.Providers["ollama"] = *provider
					p, err := providers.New(*provider)
					if err != nil {
						t.Fatal(err)
					}
					a.agent.Provider = p
					wanted := config.Model{Name: "helper-model", Provider: "ollama", Context: 8192, Think: tc.think, Generation: tc.generation}
					entry := config.GuardrailScopeEntry{Prompt: "Compaction"}
					if dedicated {
						entry = config.GuardrailScopeEntry{Agent: "Disabled"}
						a.agent.Model.Think = thinking.NewValue("high")
						a.agent.Model.Generation = &generation.Generation{MaxOutputTokens: new(999)}
						a.cfg.Agents.Apply(func(ag *config.Agent) {
							if ag.Name() == "Disabled" {
								ag.Metadata.Model = wanted
							}
						})
					} else {
						a.agent.Model = wanted
					}
					metadata := &model.Model{Name: wanted.Name}
					a.resolved.guardrail.toolCalls.model = metadata
					a.resolved.guardrail.userMessages.model = metadata
					a.resolved.titleModel = metadata
					a.resolved.thinkingModel = metadata
					switch feature {
					case "tool_calls", "user_messages":
						a.cfg.Features.Guardrail = config.Guardrail{Mode: "block", OnError: "block", Timeout: time.Second,
							Scope: config.GuardrailScope{ToolCalls: entry, UserMessages: entry}}
						blocked, _, err := a.CheckGuardrail(t.Context(), feature, "Bash", "harmless input")
						if err != nil || blocked {
							t.Fatalf("blocked=%t, err=%v", blocked, err)
						}
					case "title":
						a.cfg.Features.Title = config.Title{Agent: entry.Agent, Prompt: entry.Prompt}
						a.builder.AddUserMessage(t.Context(), "harmless input", 2)
						if _, err := a.GenerateTitle(t.Context()); err != nil {
							t.Fatal(err)
						}
					case "thinking":
						resolved, err := a.ResolveAgent(t.Context(), FeatureAgentConfig{label: "thinking", agentName: entry.Agent,
							promptName: entry.Prompt, modelCache: &a.resolved.thinkingModel})
						if err != nil {
							t.Fatal(err)
						}
						if _, err := a.CallRewrite(t.Context(), resolved, "harmless reasoning"); err != nil {
							t.Fatal(err)
						}
					}
					var body map[string]any
					select {
					case body = <-requests:
					default:
						t.Fatal("helper did not send a request")
					}
					if body["model"] != wanted.Name || body["tools"] != nil {
						t.Fatalf("wrong helper request: %v", body)
					}
					if feature == "tool_calls" || feature == "user_messages" {
						if body["response_format"] == nil {
							t.Error("guardrail lost its structured response format")
						}
					}
					if tc.name == "unset" {
						if body["chat_template_kwargs"] != nil {
							t.Error("unset thinking acquired an override")
						}
					} else {
						kwargs, _ := body["chat_template_kwargs"].(map[string]any)
						if kwargs["enable_thinking"] != (tc.name != "off") {
							t.Errorf("lost thinking override: %v", kwargs)
						}
					}
					if tc.name == "configured" {
						for key, expected := range map[string]any{"reasoning_effort": "low", "temperature": 0.2,
							"top_k": float64(20), "max_tokens": float64(64), "thinking_budget_tokens": float64(32)} {
							if body[key] != expected {
								t.Errorf("%s=%v, want %v", key, body[key], expected)
							}
						}
					} else if body["max_tokens"] != nil || body["thinking_budget_tokens"] != nil || body["temperature"] != nil {
						t.Error("unset generation acquired the caller's settings")
					}
				})
			}
		}
	}
}
