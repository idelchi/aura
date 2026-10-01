package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/invopop/jsonschema"

	"github.com/idelchi/aura/pkg/llm/message"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/llm/request"
	"github.com/idelchi/aura/pkg/llm/roles"
	"github.com/idelchi/aura/pkg/llm/tool"
	"github.com/idelchi/aura/pkg/providers"
	"github.com/idelchi/aura/pkg/providers/anthropic"
	"github.com/idelchi/aura/pkg/providers/google"
	"github.com/idelchi/aura/pkg/providers/llamacpp"
	"github.com/idelchi/aura/pkg/providers/llamaswap"
	"github.com/idelchi/aura/pkg/providers/ollama"
	"github.com/idelchi/aura/pkg/providers/openai"
)

type schemaProbe struct{ tool.Base }

func (schemaProbe) Schema() tool.Schema                                     { return tool.Schema{} }
func (schemaProbe) Execute(context.Context, map[string]any) (string, error) { return "", nil }

func (schemaProbe) Name() string { return "SchemaProbe" }

// Deliberately differ from alphabetical order at every nesting level. Optional
// fields must remain optional; order preservation must not change the contract.
type schemaChild struct {
	Zulu  string `json:"zulu" jsonschema:"enum=one,enum=two"`
	Alpha string `json:"alpha,omitempty"`
}

type schemaInput struct {
	Summary string        `json:"summary,omitempty"`
	Items   []schemaChild `json:"items,omitempty"`
	Config  schemaChild   `json:"config,omitempty"`
}

// TestToolSchemaWire catches reordering by both Aura and the provider SDKs.
// Google consumes typed maps, so test field retention there rather than order.
func TestToolSchemaWire(t *testing.T) {
	t.Parallel()
	for _, backend := range []string{"llamacpp", "llamaswap", "ollama", "bifrost-chat", "openai-responses", "anthropic", "google"} {
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			seen := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Tools []struct {
						Parameters jsonschema.Schema `json:"parameters"`
						Input      jsonschema.Schema `json:"input_schema"`
						Function   struct {
							Parameters jsonschema.Schema `json:"parameters"`
						} `json:"function"`
						Declarations []struct {
							Parameters jsonschema.Schema `json:"parameters"`
						} `json:"functionDeclarations"`
					} `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				} else if len(body.Tools) != 1 {
					t.Errorf("tools = %d, want 1", len(body.Tools))
				} else {
					definition := body.Tools[0]
					schema := definition.Function.Parameters
					switch backend {
					case "openai-responses":
						schema = definition.Parameters
					case "anthropic":
						schema = definition.Input
					case "google":
						if len(definition.Declarations) == 1 {
							schema = definition.Declarations[0].Parameters
						}
					}
					assertSchemaWire(t, &schema, backend != "google")
				}
				seen <- struct{}{}
				// Only request serialization is under test; no model is involved.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"wire inspected","type":"invalid_request_error"}}`))
			}))
			defer server.Close()
			var client providers.Provider
			name := "route/local-model"
			switch backend {
			case "llamacpp":
				client = llamacpp.New(server.URL, "test", time.Second)
			case "llamaswap":
				client = llamaswap.New(server.URL, "test", time.Second)
			case "ollama":
				var err error
				client, err = ollama.New(server.URL, "test", 0, time.Second)
				if err != nil {
					t.Fatal(err)
				}
			case "bifrost-chat", "openai-responses":
				client = openai.New(server.URL+"/v1", "test", time.Second, nil)
				if backend == "openai-responses" {
					name = "gpt-5-mini"
				}
			case "anthropic":
				client = anthropic.New(server.URL, "test", time.Second)
			case "google":
				var err error
				client, err = google.New(server.URL, "test", time.Second)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, _, err := client.Chat(t.Context(), request.Request{
				Model:    model.Model{Name: name},
				Messages: message.Messages{{Role: roles.User, Content: "Plan a task."}},
				Tools:    []tool.Schema{tool.GenerateSchema[schemaInput](schemaProbe{})},
			}, nil)
			if err == nil {
				t.Error("expected the probe server's rejection")
			}
			select {
			case <-seen:
			default:
				t.Fatalf("request never reached server: %v", err)
			}
		})
	}
}

func assertSchemaWire(t *testing.T, schema *jsonschema.Schema, ordered bool) {
	t.Helper()
	keys := slices.Collect(schema.Properties.KeysFromOldest())
	want := []string{"summary", "items", "config"}
	if !ordered {
		slices.Sort(keys)
		slices.Sort(want)
	}
	if !slices.Equal(keys, want) {
		t.Errorf("properties = %v, want %v", keys, want)
	}
	if len(schema.Required) != 0 {
		t.Errorf("optional fields became required: %v", schema.Required)
	}
	for _, name := range []string{"items", "config"} {
		child, ok := schema.Properties.Get(name)
		if !ok {
			continue // Missing field was reported above.
		}
		if name == "items" {
			child = child.Items
		}
		if child == nil {
			t.Errorf("%s lost its schema", name)
			continue
		}
		keys = slices.Collect(child.Properties.KeysFromOldest())
		want = []string{"zulu", "alpha"}
		if !ordered {
			slices.Sort(keys)
			slices.Sort(want)
		}
		if !slices.Equal(keys, want) {
			t.Errorf("%s properties = %v, want %v", name, keys, want)
		}
		if !ordered {
			continue // Google's typed SDK path is checked for field retention above.
		}
		if name == "items" && !slices.Equal(child.Required, []string{"zulu"}) {
			t.Errorf("array item required fields changed: %v", child.Required)
		}
		if zulu, ok := child.Properties.Get("zulu"); !ok || len(zulu.Enum) != 2 {
			t.Errorf("%s lost enum constraint", name)
		}
	}
}
