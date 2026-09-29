package commands

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

func TestModelIcons(t *testing.T) {
	for _, tc := range []struct {
		model model.Model
		want  string
	}{
		{model.Model{}, " T? V?"},
		{model.Model{CapabilitiesKnown: true}, ""},
		{model.Model{CapabilitiesKnown: true, Capabilities: capabilities.Capabilities{capabilities.Thinking}}, " T"},
		{model.Model{Capabilities: capabilities.Capabilities{capabilities.Vision}}, " T? V"},
	} {
		if got := modelIcons(tc.model); got != tc.want {
			t.Errorf("icons = %q, want %q", got, tc.want)
		}
	}
}
