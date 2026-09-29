package google

import (
	"testing"

	"github.com/idelchi/aura/pkg/llm/model"
)

func TestPartialMetadataStillAllowsVisionEnrichment(t *testing.T) {
	t.Parallel()
	m := model.Model{Name: "gemini-2.5-pro"}
	enrichFromAPI(&m, false, []string{"generateContent"})
	if !m.Capabilities.Vision() {
		t.Fatal("registry vision metadata was blocked by partial API metadata")
	}
	if m.Capabilities.Thinking() {
		t.Fatal("registry overwrote the API's explicit thinking restriction")
	}
	if !m.Capabilities.Tools() {
		t.Fatal("reported tools capability was lost")
	}
}
