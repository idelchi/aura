package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"
)

type copilotModelsResponse struct {
	Data []copilotModelEntry `json:"data"`
}

type copilotModelEntry struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	SupportedEndpoints []string `json:"supported_endpoints"`
	Capabilities       struct {
		Limits struct {
			MaxContextWindowTokens int `json:"max_context_window_tokens"`
			MaxOutputTokens        int `json:"max_output_tokens"`
		} `json:"limits"`
		Supports struct {
			ToolCalls       bool     `json:"tool_calls"`
			Vision          bool     `json:"vision"`
			ReasoningEffort []string `json:"reasoning_effort"`
		} `json:"supports"`
	} `json:"capabilities"`
}

type modelInfo struct {
	Model     model.Model
	Endpoints []string
}

func (c *Client) fetchModels(ctx context.Context) (model.Models, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://copilot.placeholder/models", nil)
	if err != nil {
		return nil, fmt.Errorf("building models request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching copilot models: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot models returned status %d", resp.StatusCode)
	}

	var body copilotModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding copilot models response: %w", err)
	}

	cache := make(map[string]modelInfo, len(body.Data))

	var models model.Models

	for _, entry := range body.Data {
		hasResponses := slices.Contains(entry.SupportedEndpoints, "/responses")
		hasMessages := slices.Contains(entry.SupportedEndpoints, "/v1/messages")

		if !hasResponses && !hasMessages {
			continue
		}

		m := model.Model{
			Name:           entry.ID,
			ParameterCount: model.ParseParameterName(entry.ID),
			ContextLength:  model.ContextLength(entry.Capabilities.Limits.MaxContextWindowTokens),
		}

		if entry.Capabilities.Supports.ToolCalls {
			m.Capabilities.Add(capabilities.Tools)
		}

		if entry.Capabilities.Supports.Vision {
			m.Capabilities.Add(capabilities.Vision)
		}

		if len(entry.Capabilities.Supports.ReasoningEffort) > 0 {
			m.Capabilities.Add(capabilities.Thinking)
		}

		cache[entry.ID] = modelInfo{
			Model:     m,
			Endpoints: entry.SupportedEndpoints,
		}

		models = append(models, m)
	}

	c.mu.Lock()
	c.models = cache
	c.mu.Unlock()

	return models, nil
}

func (c *Client) ensureModels(ctx context.Context) error {
	c.mu.Lock()
	cached := c.models
	c.mu.Unlock()

	if cached != nil {
		return nil
	}

	_, err := c.fetchModels(ctx)

	return err
}

// Models returns all Copilot models that support /responses or /v1/messages endpoints.
func (c *Client) Models(ctx context.Context) (model.Models, error) {
	if err := c.transport.ensureToken(ctx); err != nil {
		return nil, err
	}

	models, err := c.fetchModels(ctx)
	if err != nil {
		return nil, err
	}

	return models, nil
}
