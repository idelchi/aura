package config

import (
	"strings"
	"testing"

	"github.com/idelchi/aura/internal/prompts"
)

func TestNewToolsDataIncludesLoadToolsForDeferredTools(t *testing.T) {
	t.Parallel()

	data := struct {
		Tools ToolsData
	}{
		Tools: NewToolsData(
			[]string{"Read", "Rg"},
			"The following tools are available but not yet loaded. Use LoadTools to load them before use:\n- mcp__teams__signup\n",
		),
	}

	rendered, err := prompts.Prompt(`
{{ if .Tools.Eager -}}
Currently callable:
{{ range .Tools.Eager }}- {{ . }}
{{ end }}
{{ if not .Tools.Deferred }}No other tools exist.{{ end }}
{{ else -}}
You have NO tools available.
{{ end -}}
{{ if .Tools.Deferred }}
{{ .Tools.Deferred }}
{{ end -}}
`).Render(data)
	if err != nil {
		t.Fatal(err)
	}

	output := rendered.String()

	for _, expected := range []string{"- Read", "- Rg", "- LoadTools", "mcp__teams__signup"} {
		if !strings.Contains(output, expected) {
			t.Errorf("rendered prompt missing %q:\n%s", expected, output)
		}
	}

	if strings.Contains(output, "No other tools exist") {
		t.Errorf("rendered prompt denies deferred tools:\n%s", output)
	}
}

func TestNewToolsDataWithoutDeferredToolsDoesNotIncludeLoadTools(t *testing.T) {
	t.Parallel()

	data := NewToolsData([]string{"Read"}, "")

	if strings.Join(data.Eager, ",") != "Read" {
		t.Fatalf("unexpected eager tools: %v", data.Eager)
	}
}
