package llamacpp

import (
	"context"
	"fmt"
	"net/http"
)

// ShowResponse represents model properties from the /props endpoint.
type ShowResponse struct {
	DefaultGenerationSettings GenerationSettings `json:"default_generation_settings"`
	ChatTemplateCaps          TemplateCaps       `json:"chat_template_caps"`
	ChatTemplate              string             `json:"chat_template"`
	Modalities                Modalities         `json:"modalities"`
}

// Modalities contains supported input/output modalities.
type Modalities struct {
	Vision *bool `json:"vision"`
}

// GenerationSettings contains default generation parameters.
type GenerationSettings struct {
	ContextLength int `json:"n_ctx"`
}

// TemplateCaps contains chat template capability flags.
type TemplateCaps struct {
	SupportsReasoningEffort *bool `json:"supports_reasoning_effort"`
	SupportsToolCalls       *bool `json:"supports_tool_calls"`
}

// Show fetches model properties from the /props endpoint.
func (c *Client) Show(ctx context.Context, name string) (ShowResponse, error) {
	u, err := c.modelEndpoint(name, "/props")
	if err != nil {
		return ShowResponse{}, err
	}

	q := u.Query()
	q.Set("model", name)
	q.Add("autoload", "true")

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ShowResponse{}, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.Send(req)
	if err != nil {
		return ShowResponse{}, err
	}
	defer resp.Body.Close()

	var show ShowResponse

	if err := decodeJSON(req, resp, &show); err != nil {
		return ShowResponse{}, err
	}

	return show, nil
}
