package google

import (
	"slices"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
	"github.com/idelchi/aura/pkg/providers/registry"
)

// enrichFromAPI adds capabilities from the Google API response and runs registry enrichment.
func enrichFromAPI(mdl *model.Model, thinking bool, actions []string) {
	mdl.SetCapability(capabilities.Thinking, thinking)
	mdl.SetCapability(capabilities.ThinkingLevels, thinking)
	if actions != nil {
		mdl.SetCapability(capabilities.Tools, slices.Contains(actions, "generateContent"))
		mdl.SetCapability(capabilities.Embedding, slices.Contains(actions, "embedContent"))
	}

	registry.Enrich("google", mdl)
}
