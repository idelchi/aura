package llamaswap_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/providers/capabilities"
	"github.com/idelchi/aura/pkg/providers/llamaswap"
)

func TestNativeRoutesAndPartialMetadata(t *testing.T) {
	t.Parallel()
	const name = "organization/model:quant"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case "/prefix/models":
			_, _ = fmt.Fprintf(w, `{"data":[{"id":%q,"context_length":32768,"capabilities":{"vision":true,"function_calling":true}}]}`, name)
		case "/prefix/upstream/organization%2Fmodel:quant/props":
			_, _ = fmt.Fprint(w, `{"default_generation_settings":{"n_ctx":8192},"modalities":{"vision":true},"chat_template_caps":{"supports_preserve_reasoning":false,"supports_reasoning_effort":false,"supports_tool_calls":true},"chat_template":"template"}`)
		case "/prefix/upstream/organization%2Fmodel:quant/apply-template":
			var body struct {
				Kwargs struct {
					Enabled bool `json:"enable_thinking"`
				} `json:"chat_template_kwargs"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"prompt": fmt.Sprint(body.Kwargs.Enabled)})
		case "/prefix/upstream/organization%2Fmodel:quant/tokenize":
			var body struct{ Model, Content string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Model != name || body.Content != "hello" {
				t.Errorf("tokenize body = %+v", body)
			}
			_, _ = fmt.Fprint(w, `{"tokens":[1,2,3]}`)
		case "/prefix/api/models/unload/organization%2Fmodel:quant":
			if r.Method != http.MethodPost {
				t.Errorf("unload method = %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected route: %s %s", r.Method, r.URL.EscapedPath())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := llamaswap.New(server.URL+"/prefix", "test-token", time.Second)
	models, err := c.Models(t.Context())
	if err != nil || len(models) != 1 {
		t.Fatalf("models = %v, %v", models, err)
	}
	m := models[0]
	if m.ContextLength != 32768 || !m.Capabilities.Vision() || !m.Capabilities.Tools() || m.Knows(capabilities.Thinking) {
		t.Fatalf("partial catalog = %+v", m)
	}
	m, err = c.Model(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Capabilities.Thinking() || !m.Capabilities.Vision() || m.ContextLength != 8192 {
		t.Fatalf("props = %+v", m)
	}
	if _, err := m.NormalizeThinking(thinking.NewValue(true), false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.NormalizeThinking(thinking.NewValue("high"), false); err == nil {
		t.Fatal("boolean-only template accepted explicit effort")
	}
	n, err := c.Estimate(t.Context(), request.Request{Model: m}, "hello")
	if err != nil || n != 3 {
		t.Fatalf("estimate = %d, %v", n, err)
	}
	if err := c.UnloadModel(t.Context(), name); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadModel(t.Context(), name); err != nil {
		t.Fatal(err)
	}
}
