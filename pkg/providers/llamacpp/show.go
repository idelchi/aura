package llamacpp

import (
	"context"
	"encoding/json"
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
	Vision bool `json:"vision"`
}

// GenerationSettings contains default generation parameters.
type GenerationSettings struct {
	ContextLength int `json:"n_ctx"`
}

// TemplateCaps contains chat template capability flags.
type TemplateCaps struct {
	SupportsPreserveReasoning bool `json:"supports_preserve_reasoning"`
	SupportsToolCalls         bool `json:"supports_tool_calls"`
}

// Error represents an API error response.
type Error struct {
	// Message describes the error.
	Message string `json:"message"`
	// Type categorizes the error.
	Type string `json:"type"`
}

// Error returns the formatted error message.
func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// FromResponse parses an error from an HTTP response.
func (e *Error) FromResponse(resp *http.Response) error {
	var wrapper struct {
		Error Error `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	*e = wrapper.Error

	return nil
}

// Show fetches model properties from the /props endpoint.
func (c *Client) Show(ctx context.Context, name string) (ShowResponse, error) {
	u, err := c.WithEndpoint("/props")
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

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return ShowResponse{}, fmt.Errorf("fetching model properties: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr Error
		if err := apiErr.FromResponse(resp); err != nil {
			return ShowResponse{}, err
		}

		return ShowResponse{}, &apiErr
	}

	var show ShowResponse
	if err := json.NewDecoder(resp.Body).Decode(&show); err != nil {
		return ShowResponse{}, fmt.Errorf("decoding props: %w", err)
	}

	return show, nil
}
