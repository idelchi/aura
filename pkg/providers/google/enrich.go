package google

import (
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
	"github.com/idelchi/aura/pkg/providers/registry"
)

// enrichFromAPI adds capabilities from the Google API response and runs registry enrichment.
func enrichFromAPI(mdl *model.Model, thinking bool, actions []string) {
	if thinking {
		mdl.Capabilities.Add(capabilities.Thinking)
		mdl.Capabilities.Add(capabilities.ThinkingLevels)
	}

	for _, action := range actions {
		switch action {
		case "generateContent":
			mdl.Capabilities.Add(capabilities.Tools)
		case "embedContent":
			mdl.Capabilities.Add(capabilities.Embedding)
		}
	}

	registry.Enrich("google", mdl)
}
