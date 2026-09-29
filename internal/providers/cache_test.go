package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/pkg/cache"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

func TestModelMetadataSurvivesWorkerUnloading(t *testing.T) {
	for _, kind := range []string{"llamacpp", "llamaswap"} {
		t.Run(kind, func(t *testing.T) {
			var inspections, catalogs atomic.Int32
			var rejectTools atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/models":
					catalogs.Add(1)
					if rejectTools.Load() {
						fmt.Fprint(w, `{"data":[{"id":"example","capabilities":{"function_calling":false}}]}`)
					} else {
						fmt.Fprint(w, `{"data":[{"id":"example"},{"id":"untouched"}]}`)
					}
				case strings.HasSuffix(r.URL.Path, "/props"):
					inspections.Add(1)
					fmt.Fprint(w, `{"default_generation_settings":{"n_ctx":32768},"modalities":{"vision":true},"chat_template_caps":{"supports_tool_calls":true},"chat_template":"template"}`)
				case strings.HasSuffix(r.URL.Path, "/apply-template"):
					var body struct {
						Kwargs map[string]bool `json:"chat_template_kwargs"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					json.NewEncoder(w).Encode(map[string]string{"prompt": fmt.Sprint(body.Kwargs["enable_thinking"])})
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			home := t.TempDir()
			cfg := config.Provider{Type: kind, URL: server.URL, Timeout: "1s", Cache: cache.New(home, false)}
			models, err := CachedModels(t.Context(), cfg)
			if err != nil || len(models) != 2 || models[0].Knows(capabilities.Tools) || inspections.Load() != 0 {
				t.Fatalf("initial listing inspected models: %+v, %v", models, err)
			}
			p, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			m, err := p.Model(t.Context(), "example")
			if err != nil || !m.Capabilities.Thinking() || !m.Capabilities.Tools() {
				t.Fatalf("inspection = %+v, %v", m, err)
			}
			// New provider instance represents a later invocation, with no loaded worker.
			models, err = CachedModels(t.Context(), cfg)
			if err != nil || !models[0].Capabilities.Thinking() || !models[0].Capabilities.Vision() || models[0].ContextLength != 32768 {
				t.Fatalf("inspection not reused by cached catalog: %+v, %v", models, err)
			}
			if catalogs.Load() != 2 || inspections.Load() != 1 || models[1].Knows(capabilities.Tools) {
				t.Fatal("listing inspected a worker or invented metadata")
			}
			// Interactive listings are still live; partial reports retain learned data.
			models, err = p.Models(t.Context())
			if err != nil || !models[0].Capabilities.Thinking() || catalogs.Load() != 3 {
				t.Fatalf("live listing: %+v, %v", models, err)
			}
			rejectTools.Store(true)
			models, err = p.Models(t.Context())
			if err != nil || models[0].Capabilities.Tools() || !models[0].Knows(capabilities.Tools) {
				t.Fatalf("fresh false lost: %+v, %v", models, err)
			}
			models, err = CachedModels(t.Context(), cfg)
			if err != nil || models[0].Capabilities.Tools() || len(models) != 1 {
				t.Fatalf("stale metadata or removed model restored: %+v, %v", models, err)
			}
			rejectTools.Store(false)
			cfg.Cache = cache.New(home, true)
			models, err = CachedModels(t.Context(), cfg)
			if err != nil || models[0].Knows(capabilities.Thinking) || models[0].ContextLength != 0 {
				t.Fatalf("no-cache reused metadata: %+v, %v", models, err)
			}
			cfg.Cache = cache.New(home, false)
			// A no-cache refresh can contradict an earlier successful inspection.
			// Its explicit report must also win on the next cached invocation.
			if _, err := p.Model(t.Context(), "example"); err != nil {
				t.Fatal(err)
			}
			rejectTools.Store(true)
			cfg.Cache = cache.New(home, true)
			if _, err := CachedModels(t.Context(), cfg); err != nil {
				t.Fatal(err)
			}
			cfg.Cache = cache.New(home, false)
			models, err = CachedModels(t.Context(), cfg)
			if err != nil || models[0].Capabilities.Tools() || !models[0].Knows(capabilities.Tools) {
				t.Fatalf("no-cache report overridden by older inspection: %+v, %v", models, err)
			}
			rejectTools.Store(false)
			models, err = p.Models(t.Context())
			if err != nil || models[0].Capabilities.Tools() || !models[0].Knows(capabilities.Tools) {
				t.Fatalf("older inspection resurfaced after partial catalog: %+v, %v", models, err)
			}
			cfg.Token = "different-credential-scope"
			models, err = CachedModels(t.Context(), cfg)
			if err != nil || models[0].Knows(capabilities.Thinking) {
				t.Fatalf("metadata leaked between connections: %+v, %v", models, err)
			}
			if inspections.Load() != 2 {
				t.Fatalf("listings inspected workers %d times", inspections.Load())
			}
		})
	}
}
