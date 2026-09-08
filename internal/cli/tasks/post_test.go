package tasks

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/task"
)

// TestPostHooksOutliveCancellation runs cleanup despite a failed/cancelled task.
func TestPostHooksOutliveCancellation(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancelled", true: "expired"}[expired], func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			if expired {
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			} else {
				ctx, cancel = context.WithCancel(t.Context())
			}
			cancel()
			output := filepath.Join(t.TempDir(), "post")
			definition := task.Task{Name: "cleanup", Post: []string{"false", `printf '%s' "${VALUE}" > "${OUTPUT}"`}}
			err := runPostHooks(io.Discard, ctx, definition, false, nil, map[string]string{"VALUE": "cleaned", "OUTPUT": output})
			if err == nil {
				t.Fatal("cleanup failure not reported")
			}
			data, err := os.ReadFile(output)
			if err != nil || string(data) != "cleaned" {
				t.Fatalf("later cleanup did not run: %s %v", data, err)
			}
		})
	}
}

// TestStructuredShellTemplates preserves arbitrary model/URL values as literal shell words.
func TestStructuredShellTemplates(t *testing.T) {
	output := filepath.Join(t.TempDir(), "post")
	model := "model's $(printf unsafe); \"variant\""
	url := "http://localhost/path?a=1&b='two'"
	data := config.TemplateData{Model: config.ModelData{Name: model}, Provider: config.ProviderData{Name: "test", URL: url}}
	definition := task.Task{Name: "structured", Post: []string{`printf '%s\n%s' {{ .Model.Name | shellQuote }} {{ .Provider.URL | shellQuote }} > "${OUTPUT}"`}}
	if err := runPostHooks(io.Discard, t.Context(), definition, false, data.Context(), map[string]string{"OUTPUT": output}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil || string(got) != model+"\n"+url {
		t.Fatalf("runtime value changed during shell evaluation: %q %v", got, err)
	}
}

// TestPostDeadline bounds cleanup separately and preserves the original task error.
func TestPostDeadline(t *testing.T) {
	definition := task.Task{Name: "bounded", PostTimeout: 25 * time.Millisecond, Post: []string{"sleep 5", "true"}}
	started := time.Now()
	err := runPostHooks(io.Discard, t.Context(), definition, false, nil, nil)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("post deadline was not enforced")
	}
	original := errors.New("task failed")
	if !errors.Is(errors.Join(original, err), original) {
		t.Fatal("cleanup masked task failure")
	}
}

// TestPostTemplates uses the same runtime template and environment expansion as pre.
func TestPostTemplates(t *testing.T) {
	output := filepath.Join(t.TempDir(), "post")
	definition := task.Task{Name: "templates", Post: []string{`printf '%s' "{{.Value}}:${VALUE}" > "${OUTPUT}"`}}
	err := runPostHooks(io.Discard, t.Context(), definition, false, map[string]string{"Value": "template"}, map[string]string{"VALUE": "runtime", "OUTPUT": output})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "template:runtime" {
		t.Fatalf("template/env not expanded: %s %v", data, err)
	}
}
