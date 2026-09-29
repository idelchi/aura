package catalog

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/thinking"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

func TestPartialCatalogDoesNotRejectThinking(t *testing.T) {
	t.Parallel()
	m, err := Decode([]byte(`{"id":"private/model","context_length":32768,"capabilities":{"vision":true,"function_calling":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.ContextLength != 32768 || !m.Capabilities.Vision() || m.Capabilities.Tools() || !m.Knows(capabilities.Tools) {
		t.Fatalf("metadata lost: %+v", m)
	}
	if m.Knows(capabilities.Thinking) {
		t.Fatal("absent reasoning field was interpreted as false")
	}
	if _, err := m.NormalizeThinking(thinking.NewValue("high"), false); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitRestrictionIsAuthoritative(t *testing.T) {
	t.Parallel()
	m, err := Decode([]byte(`{"id":"private/model","capabilities":{"reasoning":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.NormalizeThinking(thinking.NewValue(true), false); err == nil {
		t.Fatal("explicit false should reject on")
	}
	if _, err := m.NormalizeThinking(thinking.NewValue("auto"), false); err != nil {
		t.Fatalf("auto should leave the model default alone: %v", err)
	}
}
