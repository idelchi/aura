package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// supportsThinking checks the template's boolean control without generating tokens
// or inspecting model names. Older servers without apply-template remain unknown.
func (c *Client) supportsThinking(ctx context.Context, name string) (bool, error) {
	on, err := c.applyThinkingTemplate(ctx, name, true)
	if err != nil {
		return false, err
	}
	off, err := c.applyThinkingTemplate(ctx, name, false)
	if err != nil {
		return false, err
	}
	return on != off, nil
}

func (c *Client) applyThinkingTemplate(ctx context.Context, name string, enabled bool) (string, error) {
	endpoint, err := c.modelEndpoint(name, "/apply-template")
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{
		"model":                name,
		"messages":             []map[string]string{{"role": "user", "content": "Hello"}},
		"chat_template_kwargs": map[string]bool{"enable_thinking": enabled},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("apply-template: %s", resp.Status)
	}
	var result struct {
		Prompt *string `json:"prompt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Prompt == nil {
		return "", fmt.Errorf("apply-template response has no prompt")
	}
	return *result.Prompt, nil
}
