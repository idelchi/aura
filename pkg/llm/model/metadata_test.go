package model

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

func TestPartialCapabilityDisplay(t *testing.T) {
	unknown := Model{Name: "unreported"}
	plain := Model{Name: "plain", CapabilitiesKnown: true}
	tools := Model{Name: "tools"}
	tools.SetCapability(capabilities.Tools, true)
	tools.SetCapability(capabilities.Vision, true)
	embed := Model{Name: "embed", Capabilities: capabilities.Capabilities{capabilities.Embedding}}
	models := Models{unknown, plain, tools, embed}
	if got := models.IsGeneral().Names(); len(got) != 1 || got[0] != "plain" {
		t.Fatalf("other models = %v", got)
	}
	if got := models.UnknownTools().Names(); len(got) != 1 || got[0] != "unreported" {
		t.Fatalf("unknown models = %v", got)
	}
	if got := models.WithCapability(capabilities.Thinking); len(got) != 0 {
		t.Fatalf("unknown thinking passed filter: %v", got)
	}
	for _, tc := range []struct {
		model       Model
		cap         capabilities.Capability
		label, want string
	}{
		{unknown, capabilities.Thinking, "T", "T?"},
		{plain, capabilities.Thinking, "T", ""},
		{tools, capabilities.Vision, "V", "V"},
	} {
		if got := tc.model.CapabilityIndicator(tc.cap, tc.label); got != tc.want {
			t.Errorf("indicator = %q, want %q", got, tc.want)
		}
	}
	var out bytes.Buffer
	if err := models.Display(&out, SortByName); err != nil {
		t.Fatal(err)
	}
	text := ansi.Strip(out.String())
	_, unknownSection, found := strings.Cut(text, "Tool capability unknown:")
	if !found || !strings.Contains(unknownSection, "unreported") || !strings.Contains(unknownSection, "T? V?") {
		t.Fatalf("missing unknown section/indicators: %s", text)
	}
}

func TestFillMissingMetadata(t *testing.T) {
	previous := Model{Name: "example", ContextLength: 32768, Size: 123, CapabilitiesKnown: true,
		Capabilities: capabilities.Capabilities{capabilities.Tools, capabilities.Thinking}}
	fresh := Model{Name: "example", ContextLength: 8192}
	fresh.SetCapability(capabilities.Tools, false)
	merged := fresh.FillMissing(previous)
	if merged.Capabilities.Tools() || !merged.Knows(capabilities.Tools) || !merged.Capabilities.Thinking() {
		t.Fatalf("fresh false or saved thinking lost: %+v", merged)
	}
	if !merged.Knows(capabilities.Vision) || merged.Capabilities.Vision() || merged.ContextLength != 8192 || merged.Size != 123 {
		t.Fatalf("metadata = %+v", merged)
	}
	if fresh.Knows(capabilities.Thinking) || !previous.Capabilities.Tools() {
		t.Fatal("merge mutated its inputs")
	}
	if other := (Model{Name: "different"}).FillMissing(previous); other.Knows(capabilities.Tools) || other.Size != 0 {
		t.Fatalf("merged another model: %+v", other)
	}
	if explicit := (Model{Name: "example", CapabilitiesKnown: true}).FillMissing(previous); explicit.Capabilities.Thinking() {
		t.Fatal("overrode an authoritative unsupported capability")
	}
}
