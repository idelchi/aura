package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

const codexClientVersion = "0.99.0"

type reasoningLevel struct {
	Effort string `json:"effort"`
}

type codexModel struct {
	Slug                      string           `json:"slug"`
	DisplayName               string           `json:"display_name"`
	ContextWindow             int              `json:"context_window"`
	InputModalities           []string         `json:"input_modalities"`
	SupportsParallelToolCalls bool             `json:"supports_parallel_tool_calls"`
	SupportedReasoningLevels  []reasoningLevel `json:"supported_reasoning_levels"`
}

type codexModelsResponse struct {
	Models []codexModel `json:"models"`
}

// Models fetches available Codex models from the ChatGPT backend.
func (c *Client) Models(ctx context.Context) (model.Models, error) {
	modelsURL := strings.TrimSuffix(c.baseURL, "/") + "/models?client_version=" + codexClientVersion

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating models request: %w", err)
	}

	resp, err := c.inner.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching codex models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("codex models endpoint returned status %d", resp.StatusCode)
	}

	var result codexModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding models response: %w", err)
	}

	models := make(model.Models, 0, len(result.Models))

	for _, m := range result.Models {
		var caps capabilities.Capabilities

		if m.SupportsParallelToolCalls {
			caps.Add(capabilities.Tools)
		}

		if slices.Contains(m.InputModalities, "image") {
			caps.Add(capabilities.Vision)
		}

		if len(m.SupportedReasoningLevels) > 0 {
			caps.Add(capabilities.Thinking)
		}

		models = append(models, model.Model{
			Name:           m.Slug,
			ParameterCount: model.ParseParameterName(m.Slug),
			ContextLength:  model.ContextLength(m.ContextWindow),
			Capabilities:   caps,
		})
	}

	return models, nil
}
