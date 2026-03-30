package slash

import "github.com/idelchi/aura/pkg/llm/model"

// ProviderModels holds the result of querying a single provider for its models.
type ProviderModels struct {
	Name   string
	Models model.Models
	Err    error
}
